package store

import (
	"context"
	"regexp"
	"strings"
	"time"

	"gorm.io/gorm"
)

// voteSpeedSampleLimit caps how much history the dynamic vote-speed
// calibration reads. Recent votes describe the voter best and keep the query
// cheap for very active accounts.
const voteSpeedSampleLimit = 1000

func (s *Store) ListVoteModes(ctx context.Context) ([]VoteMode, error) {
	var rows []voteModeRow
	if err := s.dbWithContext(ctx).Order("id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]VoteMode, 0, len(rows))
	for i := range rows {
		out = append(out, *rows[i].toModel())
	}
	return out, nil
}

func (s *Store) GetVoteModeByName(ctx context.Context, name string) (*VoteMode, error) {
	var row voteModeRow
	err := s.dbWithContext(ctx).Where("name = ? COLLATE NOCASE", name).Take(&row).Error
	if err != nil {
		return nil, translateError(err)
	}
	return row.toModel(), nil
}

func (s *Store) GetVoteModeByID(ctx context.Context, id int64) (*VoteMode, error) {
	var row voteModeRow
	if err := s.dbWithContext(ctx).First(&row, "id = ?", id).Error; err != nil {
		return nil, translateError(err)
	}
	return row.toModel(), nil
}

func (s *Store) CreateVoteMode(ctx context.Context, m *VoteMode) error {
	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now().UTC()
	}
	row := voteModeToRow(m)
	row.ID = 0
	if err := s.dbWithContext(ctx).Create(row).Error; err != nil {
		return translateError(err)
	}
	m.ID = row.ID
	return nil
}

func (s *Store) UpdateVoteMode(ctx context.Context, m *VoteMode) error {
	err := s.dbWithContext(ctx).Model(&voteModeRow{}).
		Where("id = ?", m.ID).
		Updates(map[string]any{
			"name":        m.Name,
			"description": m.Description,
			"min_value":   m.MinValue,
			"max_value":   m.MaxValue,
			"is_binary":   boolInt64(m.IsBinary),
		}).Error
	return translateError(err)
}

func (s *Store) CountVotesForMode(ctx context.Context, id int64) (int, error) {
	var n int64
	if err := s.dbWithContext(ctx).Model(&voteRow{}).Where("mode_id = ?", id).Count(&n).Error; err != nil {
		return 0, err
	}
	return int(n), nil
}

func (s *Store) DeleteVoteMode(ctx context.Context, id int64) error {
	return s.dbWithContext(ctx).Where("id = ?", id).Delete(&voteModeRow{}).Error
}

// --- votes ---------------------------------------------------------------

func (s *Store) CreateVote(ctx context.Context, v *Vote) error {
	if v.CreatedAt.IsZero() {
		v.CreatedAt = time.Now().UTC()
	}
	if v.Metadata == "" {
		v.Metadata = "{}"
	}
	row := voteToRow(v)
	row.ID = 0
	if err := s.dbWithContext(ctx).Create(row).Error; err != nil {
		return err
	}
	v.ID = row.ID
	return nil
}

func (s *Store) GetVote(ctx context.Context, id int64) (*Vote, error) {
	var row voteRow
	if err := s.dbWithContext(ctx).First(&row, "id = ?", id).Error; err != nil {
		return nil, translateError(err)
	}
	return row.toModel(), nil
}

// UpdateVote persists every mutable column of v.
func (s *Store) UpdateVote(ctx context.Context, v *Vote) error {
	err := s.dbWithContext(ctx).Model(&voteRow{}).
		Where("id = ?", v.ID).
		Updates(map[string]any{
			"mode_id":         v.ModeID,
			"mode_name":       v.ModeName,
			"left_image_id":   v.LeftImageID,
			"right_image_id":  v.RightImageID,
			"winner_image_id": v.WinnerImageID,
			"loser_image_id":  v.LoserImageID,
			"value":           v.Value,
			"min_value":       v.MinValue,
			"max_value":       v.MaxValue,
			"weight":          v.Weight,
			"metadata":        v.Metadata,
			"duration_ms":     v.DurationMs,
			"vote_speed":      v.VoteSpeed,
		}).Error
	return translateError(err)
}

// VoteDurationHistory returns how many votes an identity has cast and the
// durations of those votes (newest first, capped). It is the basis for the
// dynamic vote-speed calibration.
func (s *Store) VoteDurationHistory(ctx context.Context, userID *int64, anonymousID string, excludeVoteID int64) (int, []int64, error) {
	base := func() *gorm.DB {
		q := s.dbWithContext(ctx).Model(&voteRow{})
		if excludeVoteID > 0 {
			q = q.Where("id <> ?", excludeVoteID)
		}
		if userID != nil {
			return q.Where("user_id = ?", *userID)
		}
		if anonymousID == "" {
			return q.Where("1 = 0")
		}
		return q.Where("anonymous_id = ? AND user_id IS NULL", anonymousID)
	}

	var total int64
	if err := base().Count(&total).Error; err != nil {
		return 0, nil, err
	}

	var rows []voteRow
	if err := base().
		Select("duration_ms").
		Where("duration_ms IS NOT NULL").
		Order("created_at DESC, id DESC").
		Limit(voteSpeedSampleLimit).
		Find(&rows).Error; err != nil {
		return 0, nil, err
	}
	durations := make([]int64, 0, len(rows))
	for i := range rows {
		if rows[i].DurationMs != nil {
			durations = append(durations, *rows[i].DurationMs)
		}
	}
	return int(total), durations, nil
}

// ClaimAnonymousVotes moves votes cast from a device-local anonymous identity
// under a real account. Only unowned votes are touched, so a claim token that
// leaks cannot steal somebody else's history.
func (s *Store) ClaimAnonymousVotes(ctx context.Context, userID int64, username string, anonymousIDs []string) (int64, error) {
	if len(anonymousIDs) == 0 {
		return 0, nil
	}
	var claimed int64
	err := s.dbWithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, anonymousID := range anonymousIDs {
			res := tx.Model(&voteRow{}).
				Where("anonymous_id = ? AND anonymous_id <> '' AND user_id IS NULL", anonymousID).
				Updates(map[string]any{"user_id": userID, "username": username})
			if res.Error != nil {
				return res.Error
			}
			claimed += res.RowsAffected
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return claimed, nil
}

func (s *Store) DeleteVote(ctx context.Context, id int64) error {
	res := s.dbWithContext(ctx).Where("id = ?", id).Delete(&voteRow{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteVotesByIDs(ctx context.Context, ids []int64) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	var deleted int64
	err := s.dbWithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for start := 0; start < len(ids); start += 400 {
			end := min(start+400, len(ids))
			res := tx.Where("id IN ?", ids[start:end]).Delete(&voteRow{})
			if res.Error != nil {
				return res.Error
			}
			deleted += res.RowsAffected
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return deleted, nil
}

// ListVotes returns matching votes newest first plus the total count before
// pagination. Author/TitleRegex filters need image metadata and are applied
// in Go, which keeps regex semantics identical across SQLite builds.
func (s *Store) ListVotes(ctx context.Context, f VoteFilter) ([]Vote, int, error) {
	var re *regexp.Regexp
	if pattern := strings.TrimSpace(f.TitleRegex); pattern != "" {
		var err error
		re, err = regexp.Compile("(?i)" + pattern)
		if err != nil {
			return nil, 0, ErrInvalidFilter
		}
	}
	complexFilter := re != nil || strings.TrimSpace(f.Author) != ""
	fetchAll := f.Limit < 0
	defaultLimit := 100
	if f.Limit > 0 {
		defaultLimit = clampLimit(f.Limit, 100)
	}

	query := func() *gorm.DB {
		return buildVoteQuery(s.dbWithContext(ctx), f)
	}

	if !complexFilter {
		var total int64
		if err := query().Count(&total).Error; err != nil {
			return nil, 0, err
		}
		q := query().Order("created_at DESC, id DESC")
		if !fetchAll {
			q = q.Limit(defaultLimit).Offset(max(f.Offset, 0))
		}
		votes, err := findVotes(q)
		return votes, int(total), err
	}

	votes, err := findVotes(query().Order("created_at DESC, id DESC"))
	if err != nil {
		return nil, 0, err
	}

	ids := make([]string, 0, len(votes)*2)
	for _, v := range votes {
		ids = append(ids, v.LeftImageID, v.RightImageID)
	}
	// Deleted images are intentionally included here: the vote history must
	// still be filterable by the author/title it referenced at the time.
	imgs, err := s.getAllImagesByIDs(ctx, ids)
	if err != nil {
		return nil, 0, err
	}

	author := strings.ToLower(strings.TrimSpace(f.Author))
	filtered := make([]Vote, 0, len(votes))
	for _, v := range votes {
		left, right := imgs[v.LeftImageID], imgs[v.RightImageID]
		if author != "" && !authorMatches(left, right, author) {
			continue
		}
		if re != nil && !re.MatchString(titleOf(left)) && !re.MatchString(titleOf(right)) {
			continue
		}
		filtered = append(filtered, v)
	}

	total := len(filtered)
	if fetchAll {
		return filtered, total, nil
	}
	offset := max(f.Offset, 0)
	if offset >= total {
		return []Vote{}, total, nil
	}
	filtered = filtered[offset:]
	if defaultLimit < len(filtered) {
		filtered = filtered[:defaultLimit]
	}
	return filtered, total, nil
}

func findVotes(q *gorm.DB) ([]Vote, error) {
	var rows []voteRow
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]Vote, 0, len(rows))
	for i := range rows {
		out = append(out, *rows[i].toModel())
	}
	return out, nil
}

func buildVoteQuery(db *gorm.DB, f VoteFilter) *gorm.DB {
	q := db.Model(&voteRow{})
	if f.From != nil {
		q = q.Where("created_at >= ?", *f.From)
	}
	if f.To != nil {
		q = q.Where("created_at <= ?", *f.To)
	}
	if username := strings.TrimSpace(f.Username); username != "" {
		q = q.Where("username = ? COLLATE NOCASE", username)
	}
	if mode := strings.TrimSpace(f.Mode); mode != "" {
		q = q.Where("(mode_name = ? COLLATE NOCASE OR CAST(mode_id AS TEXT) = ?)", mode, mode)
	}
	if imageID := strings.TrimSpace(f.ImageID); imageID != "" {
		q = q.Where("(left_image_id = ? OR right_image_id = ?)", imageID, imageID)
	}
	return q
}

func authorMatches(left, right *Image, author string) bool {
	if left != nil && strings.Contains(strings.ToLower(left.Author), author) {
		return true
	}
	if right != nil && strings.Contains(strings.ToLower(right.Author), author) {
		return true
	}
	return false
}

func titleOf(img *Image) string {
	if img == nil {
		return ""
	}
	return img.Title
}