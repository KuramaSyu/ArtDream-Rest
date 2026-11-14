package store

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"strings"
	"time"
)

const imageColumns = `id, author, title, original_filename, storage_path, mime_type,
	width, height, size_bytes, uploaded_by, claim_token, created_at, deleted_at`

func scanImage(row interface{ Scan(...any) error }) (*Image, error) {
	var img Image
	var created int64
	var uploadedBy, deletedAt sql.NullInt64
	if err := row.Scan(&img.ID, &img.Author, &img.Title, &img.OriginalFilename, &img.StoragePath,
		&img.MimeType, &img.Width, &img.Height, &img.SizeBytes, &uploadedBy, &img.ClaimToken, &created, &deletedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	img.CreatedAt = unixToTime(created)
	if uploadedBy.Valid {
		v := uploadedBy.Int64
		img.UploadedBy = &v
	}
	if deletedAt.Valid {
		t := unixToTime(deletedAt.Int64)
		img.DeletedAt = &t
	}
	return &img, nil
}

func (s *Store) CreateImage(ctx context.Context, img *Image) error {
	if img.CreatedAt.IsZero() {
		img.CreatedAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO images (id, author, title, original_filename, storage_path, mime_type,
		 width, height, size_bytes, uploaded_by, claim_token, created_at, deleted_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL)`,
		img.ID, img.Author, img.Title, img.OriginalFilename, img.StoragePath, img.MimeType,
		img.Width, img.Height, img.SizeBytes, img.UploadedBy, img.ClaimToken, timeToUnix(img.CreatedAt))
	return err
}

// GetImage returns a live (non-deleted) image.
func (s *Store) GetImage(ctx context.Context, id string) (*Image, error) {
	return scanImage(s.db.QueryRowContext(ctx,
		`SELECT `+imageColumns+` FROM images WHERE id = ? AND deleted_at IS NULL`, id))
}

// GetImagesByIDs loads live images in one query. Missing ids are omitted.
func (s *Store) GetImagesByIDs(ctx context.Context, ids []string) (map[string]*Image, error) {
	out := make(map[string]*Image, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	placeholders := placeholders(len(ids))
	args := make([]any, 0, len(ids))
	for _, id := range ids {
		args = append(args, id)
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+imageColumns+` FROM images WHERE deleted_at IS NULL AND id IN (`+placeholders+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		img, err := scanImage(rows)
		if err != nil {
			return nil, err
		}
		out[img.ID] = img
	}
	return out, rows.Err()
}

// ImagesByAuthor lists an author's images, newest first.
func (s *Store) ImagesByAuthor(ctx context.Context, author string, limit, offset int) ([]Image, int, error) {
	var total int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM images WHERE deleted_at IS NULL AND author = ? COLLATE NOCASE`, author,
	).Scan(&total); err != nil {
		return nil, 0, err
	}
	limit = clampLimit(limit, 100)
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+imageColumns+` FROM images
		 WHERE deleted_at IS NULL AND author = ? COLLATE NOCASE
		 ORDER BY created_at DESC, id ASC LIMIT ? OFFSET ?`, author, limit, max(offset, 0))
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := make([]Image, 0)
	for rows.Next() {
		img, err := scanImage(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *img)
	}
	return out, total, rows.Err()
}

// RandomImages picks up to amount live images. exclude removes ids (useful for
// the ranking page so users do not immediately see the same pair again).
func (s *Store) RandomImages(ctx context.Context, amount int, author string, exclude []string) ([]Image, error) {
	amount = clampAmount(amount, 50)
	query := `SELECT ` + imageColumns + ` FROM images WHERE deleted_at IS NULL`
	args := make([]any, 0)
	if strings.TrimSpace(author) != "" {
		query += ` AND author = ? COLLATE NOCASE`
		args = append(args, author)
	}
	exclude = compactUnique(exclude)
	if len(exclude) > 0 {
		query += ` AND id NOT IN (` + placeholders(len(exclude)) + `)`
		for _, id := range exclude {
			args = append(args, id)
		}
	}
	query += ` ORDER BY RANDOM() LIMIT ?`
	args = append(args, amount)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Image, 0, amount)
	for rows.Next() {
		img, err := scanImage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *img)
	}
	return out, rows.Err()
}

// ListImages powers the admin image manager. Regex filtering happens in Go so
// the deployed SQLite build needs no custom REGEXP function.
func (s *Store) ListImages(ctx context.Context, f ImageFilter) ([]Image, int, error) {
	query := `SELECT ` + imageColumns + ` FROM images WHERE 1=1`
	args := make([]any, 0)
	if !f.IncludeDeleted {
		query += ` AND deleted_at IS NULL`
	}
	if author := strings.TrimSpace(f.Author); author != "" {
		query += ` AND author LIKE ? COLLATE NOCASE`
		args = append(args, "%"+author+"%")
	}
	query += ` ORDER BY created_at DESC, id ASC`

	var re *regexp.Regexp
	if pattern := strings.TrimSpace(f.TitleRegex); pattern != "" {
		var err error
		re, err = regexp.Compile("(?i)" + pattern)
		if err != nil {
			return nil, 0, ErrInvalidFilter
		}
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	all := make([]Image, 0)
	for rows.Next() {
		img, err := scanImage(rows)
		if err != nil {
			return nil, 0, err
		}
		if re != nil && !re.MatchString(img.Title) {
			continue
		}
		all = append(all, *img)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
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
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	claimed := make([]string, 0, len(claims))
	for _, claim := range claims {
		res, err := tx.ExecContext(ctx,
			`UPDATE images SET uploaded_by = ?, claim_token = '' 
			 WHERE id = ? AND claim_token = ? AND claim_token <> '' AND uploaded_by IS NULL`,
			userID, claim.ID, claim.Token)
		if err != nil {
			return nil, err
		}
		if n, _ := res.RowsAffected(); n > 0 {
			claimed = append(claimed, claim.ID)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return claimed, nil
}

// GetImagesIncludingDeleted is used by admin vote views so historical votes
// still show the image they referenced even after deletion.
func (s *Store) GetImagesIncludingDeleted(ctx context.Context, ids []string) (map[string]*Image, error) {
	return s.getAllImagesByIDs(ctx, ids)
}

// ErrInvalidFilter signals a malformed filter (e.g. bad regex).
var ErrInvalidFilter = errors.New("invalid filter")

func (s *Store) UpdateImage(ctx context.Context, id string, author, title *string) error {
	if author != nil {
		if _, err := s.db.ExecContext(ctx, `UPDATE images SET author = ? WHERE id = ?`, *author, id); err != nil {
			return err
		}
	}
	if title != nil {
		if _, err := s.db.ExecContext(ctx, `UPDATE images SET title = ? WHERE id = ?`, *title, id); err != nil {
			return err
		}
	}
	return nil
}

// DeleteImage hard-deletes the row. The caller removes files and, when
// requested, associated votes.
func (s *Store) DeleteImage(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM images WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteVotesForImage(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM votes WHERE left_image_id = ? OR right_image_id = ?`, id, id)
	return err
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

func clampAmount(v, maxV int) int {
	if v <= 0 {
		return 1
	}
	if v > maxV {
		return maxV
	}
	return v
}

func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	parts := make([]string, n)
	for i := range parts {
		parts[i] = "?"
	}
	return strings.Join(parts, ",")
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
