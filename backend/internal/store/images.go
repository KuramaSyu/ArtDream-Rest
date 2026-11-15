package store

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	"gorm.io/gorm"
)

// ErrInvalidFilter signals a malformed filter (e.g. bad regex).
var ErrInvalidFilter = errors.New("invalid filter")

func (s *Store) CreateImage(ctx context.Context, img *Image) error {
	if img.CreatedAt.IsZero() {
		img.CreatedAt = time.Now().UTC()
	}
	return s.dbWithContext(ctx).Create(imageToRow(img)).Error
}

// GetImage returns a live (non-deleted) image.
func (s *Store) GetImage(ctx context.Context, id string) (*Image, error) {
	var row imageRow
	err := s.dbWithContext(ctx).Where("id = ? AND deleted_at IS NULL", id).Take(&row).Error
	if err != nil {
		return nil, translateError(err)
	}
	return row.toModel(), nil
}

// GetImagesByIDs loads live images in batches. Missing ids are omitted. The
// input is de-duplicated and split into chunks so large vote/ranking windows
// stay below SQLite's SQL-variable limit.
func (s *Store) GetImagesByIDs(ctx context.Context, ids []string) (map[string]*Image, error) {
	return s.loadImagesByIDs(ctx, ids, false)
}

func (s *Store) loadImagesByIDs(ctx context.Context, ids []string, includeDeleted bool) (map[string]*Image, error) {
	ids = compactUnique(ids)
	out := make(map[string]*Image, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	for start := 0; start < len(ids); start += imageIDQueryBatchSize {
		end := start + imageIDQueryBatchSize
		if end > len(ids) {
			end = len(ids)
		}
		if err := s.loadImageBatch(ctx, ids[start:end], includeDeleted, out); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (s *Store) loadImageBatch(ctx context.Context, ids []string, includeDeleted bool, out map[string]*Image) error {
	query := s.dbWithContext(ctx).Where("id IN ?", ids)
	if !includeDeleted {
		query = query.Where("deleted_at IS NULL")
	}
	var rows []imageRow
	if err := query.Find(&rows).Error; err != nil {
		return err
	}
	for i := range rows {
		img := rows[i].toModel()
		out[img.ID] = img
	}
	return nil
}

// ImagesByAuthor lists an author's images, newest first. Images hidden by the
// configured allow/deny lists are omitted (and not counted in the total).
func (s *Store) ImagesByAuthor(ctx context.Context, author string, limit, offset int, policy *VisibilityPolicy) ([]Image, int, error) {
	query := s.dbWithContext(ctx).Model(&imageRow{}).Where("deleted_at IS NULL")
	if strings.TrimSpace(author) != "" {
		query = query.Where("author = ? COLLATE NOCASE", author)
	}
	query = policy.apply(query)
	query = query.Order("created_at DESC, id ASC")

	var rows []imageRow
	if err := query.Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	all := make([]Image, 0, len(rows))
	for i := range rows {
		img := rows[i].toModel()
		if policy.Hidden(img) {
			continue
		}
		all = append(all, *img)
	}
	total := len(all)
	offset = max(offset, 0)
	if offset >= total {
		return []Image{}, total, nil
	}
	all = all[offset:]
	limit = clampLimit(limit, 100)
	if limit < len(all) {
		all = all[:limit]
	}
	return all, total, nil
}

// RandomImages picks up to amount live images. exclude removes ids (useful for
// the ranking page so users do not immediately see the same pair again).
// Images hidden by the allow/deny lists are filtered out.
func (s *Store) RandomImages(ctx context.Context, amount int, author string, exclude []string, policy *VisibilityPolicy) ([]Image, error) {
	amount = clampAmount(amount, 50)
	query := s.dbWithContext(ctx).Model(&imageRow{}).Where("deleted_at IS NULL")
	if strings.TrimSpace(author) != "" {
		query = query.Where("author = ? COLLATE NOCASE", author)
	}
	query = policy.apply(query)
	exclude = compactUnique(exclude)
	if len(exclude) > 0 {
		query = query.Where("id NOT IN ?", exclude)
	}
	query = query.Order("RANDOM()")
	fetch := amount
	if !policy.Empty() {
		// Fetch extra candidates so the hidden rows do not starve the pool.
		fetch = clampAmount(amount*10, 500)
	}
	query = query.Limit(fetch)

	var rows []imageRow
	if err := query.Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]Image, 0, amount)
	for i := range rows {
		img := rows[i].toModel()
		if policy.Hidden(img) {
			continue
		}
		out = append(out, *img)
		if len(out) >= amount {
			break
		}
	}
	return out, nil
}

// ListImages powers the admin image manager. Regex filtering happens in Go so
// the deployed SQLite build needs no custom REGEXP function.
func (s *Store) ListImages(ctx context.Context, f ImageFilter) ([]Image, int, error) {
	query := s.dbWithContext(ctx).Model(&imageRow{})
	if !f.IncludeDeleted {
		query = query.Where("deleted_at IS NULL")
	}
	if author := strings.TrimSpace(f.Author); author != "" {
		query = query.Where("author LIKE ? COLLATE NOCASE", "%"+author+"%")
	}
	switch strings.ToLower(strings.TrimSpace(f.Visibility)) {
	case VisibilityPublic:
		query = query.Where("from_public_service = ?", 1)
	case VisibilityPrivate:
		query = query.Where("from_public_service = ?", 0)
	}
	if service := strings.TrimSpace(f.Service); service != "" {
		query = query.Where("LOWER(service_name) LIKE ?", "%"+strings.ToLower(service)+"%")
	}
	if len(f.UploadedBy) > 0 {
		ids := compactUniqueInt64(f.UploadedBy)
		if len(ids) > 0 {
			query = query.Where("uploaded_by IN ?", ids)
		}
	}
	query = query.Order("created_at DESC, id ASC")

	var re *regexp.Regexp
	if pattern := strings.TrimSpace(f.TitleRegex); pattern != "" {
		var err error
		re, err = regexp.Compile("(?i)" + pattern)
		if err != nil {
			return nil, 0, ErrInvalidFilter
		}
	}

	var rows []imageRow
	if err := query.Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	all := make([]Image, 0, len(rows))
	for i := range rows {
		img := rows[i].toModel()
		if re != nil && !re.MatchString(img.Title) {
			continue
		}
		all = append(all, *img)
	}

	total := len(all)
	offset := max(f.Offset, 0)
	if offset >= total {
		return []Image{}, total, nil
	}
	all = all[offset:]
	if f.Limit > 0 && f.Limit < len(all) {
		all = all[:f.Limit]
	}
	return all, total, nil
}

// ImageClaim is a secret token proving an anonymous uploader owns an image.
type ImageClaim struct {
	ID    string `json:"id"`
	Token string `json:"token"`
}

// ClaimImages links anonymous uploads to an account. The claim token prevents
// someone else from attaching an image just by knowing its public id.
func (s *Store) ClaimImages(ctx context.Context, userID int64, claims []ImageClaim) ([]string, error) {
	if len(claims) == 0 {
		return []string{}, nil
	}
	claimed := make([]string, 0, len(claims))
	err := s.dbWithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, claim := range claims {
			res := tx.Model(&imageRow{}).
				Where("id = ? AND claim_token = ? AND claim_token <> '' AND uploaded_by IS NULL",
					claim.ID, claim.Token).
				Updates(map[string]any{"uploaded_by": userID, "claim_token": ""})
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected > 0 {
				claimed = append(claimed, claim.ID)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return claimed, nil
}

// GetImagesIncludingDeleted is used by admin vote views so historical votes
// still show the image they referenced even after deletion.
func (s *Store) GetImagesIncludingDeleted(ctx context.Context, ids []string) (map[string]*Image, error) {
	return s.getAllImagesByIDs(ctx, ids)
}

func (s *Store) getAllImagesByIDs(ctx context.Context, ids []string) (map[string]*Image, error) {
	return s.loadImagesByIDs(ctx, ids, true)
}

func (s *Store) UpdateImage(ctx context.Context, id string, author, title *string, fromPublicService *bool, serviceName *string) error {
	updates := map[string]any{}
	if author != nil {
		updates["author"] = *author
	}
	if title != nil {
		updates["title"] = *title
	}
	if fromPublicService != nil {
		updates["from_public_service"] = boolInt64(*fromPublicService)
	}
	if serviceName != nil {
		updates["service_name"] = *serviceName
	}
	if len(updates) == 0 {
		return nil
	}
	return s.dbWithContext(ctx).Model(&imageRow{}).Where("id = ?", id).Updates(updates).Error
}

// DeleteImage hard-deletes the row. The caller removes files and, when
// requested, associated votes.
func (s *Store) DeleteImage(ctx context.Context, id string) error {
	res := s.dbWithContext(ctx).Where("id = ?", id).Delete(&imageRow{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteVotesForImage(ctx context.Context, id string) error {
	return s.dbWithContext(ctx).
		Where("left_image_id = ? OR right_image_id = ?", id, id).
		Delete(&voteRow{}).Error
}

func clampLimit(v, fallback int) int {
	if v <= 0 {
		return fallback
	}
	if v > 500 {
		return 500
	}
	return v
}

const imageIDQueryBatchSize = 500

func clampAmount(v, maxV int) int {
	if v <= 0 {
		return 1
	}
	if v > maxV {
		return maxV
	}
	return v
}

func compactUniqueInt64(in []int64) []int64 {
	seen := make(map[int64]struct{}, len(in))
	out := make([]int64, 0, len(in))
	for _, v := range in {
		if v <= 0 {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

func compactUnique(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, v := range in {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}