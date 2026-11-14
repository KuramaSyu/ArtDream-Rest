package store

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"strings"
	"time"
)

const voteModeColumns = `id, name, description, min_value, max_value, is_binary, created_at`

func scanVoteMode(row interface{ Scan(...any) error }) (*VoteMode, error) {
	var m VoteMode
	var created int64
	var binary int
	if err := row.Scan(&m.ID, &m.Name, &m.Description, &m.MinValue, &m.MaxValue, &binary, &created); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	m.IsBinary = binary != 0
	m.CreatedAt = unixToTime(created)
	return &m, nil
}

func (s *Store) ListVoteModes(ctx context.Context) ([]VoteMode, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+voteModeColumns+` FROM vote_modes ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]VoteMode, 0)
	for rows.Next() {
		m, err := scanVoteMode(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *m)
	}
	return out, rows.Err()
}

func (s *Store) GetVoteModeByName(ctx context.Context, name string) (*VoteMode, error) {
	return scanVoteMode(s.db.QueryRowContext(ctx,
		`SELECT `+voteModeColumns+` FROM vote_modes WHERE name = ? COLLATE NOCASE`, name))
}

func (s *Store) GetVoteModeByID(ctx context.Context, id int64) (*VoteMode, error) {
	return scanVoteMode(s.db.QueryRowContext(ctx,
		`SELECT `+voteModeColumns+` FROM vote_modes WHERE id = ?`, id))
}

func (s *Store) CreateVoteMode(ctx context.Context, m *VoteMode) error {
	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now().UTC()
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO vote_modes (name, description, min_value, max_value, is_binary, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		m.Name, m.Description, m.MinValue, m.MaxValue, boolInt(m.IsBinary), timeToUnix(m.CreatedAt))
	if err != nil {
		if isUniqueViolation(err) {
			return ErrConflict
		}
		return err
	}
	m.ID, _ = res.LastInsertId()
	return nil
}

func (s *Store) UpdateVoteMode(ctx context.Context, m *VoteMode) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE vote_modes SET name = ?, description = ?, min_value = ?, max_value = ?, is_binary = ? WHERE id = ?`,
		m.Name, m.Description, m.MinValue, m.MaxValue, boolInt(m.IsBinary), m.ID)
	if err != nil && isUniqueViolation(err) {
		return ErrConflict
	}
	return err
}

func (s *Store) CountVotesForMode(ctx context.Context, id int64) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM votes WHERE mode_id = ?`, id).Scan(&n)
	return n, err
}

func (s *Store) DeleteVoteMode(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM vote_modes WHERE id = ?`, id)
	return err
}

// --- votes ---------------------------------------------------------------

const voteColumns = `id, mode_id, mode_name, left_image_id, right_image_id,
	winner_image_id, loser_image_id, value, min_value, max_value, weight,
	user_id, username, metadata, duration_ms, created_at`

func scanVote(row interface{ Scan(...any) error }) (*Vote, error) {
	var v Vote
	var winner, loser sql.NullString
	var value, duration, userID sql.NullInt64
	var created int64
	if err := row.Scan(&v.ID, &v.ModeID, &v.ModeName, &v.LeftImageID, &v.RightImageID,
		&winner, &loser, &value, &v.MinValue, &v.MaxValue, &v.Weight,
		&userID, &v.Username, &v.Metadata, &duration, &created); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if winner.Valid {
		v.WinnerImageID = &winner.String
	}
	if loser.Valid {
		v.LoserImageID = &loser.String
	}
	if value.Valid {
		n := int(value.Int64)
		v.Value = &n
	}
	if userID.Valid {
		id := userID.Int64
		v.UserID = &id
	}
	if duration.Valid {
		d := duration.Int64
		v.DurationMs = &d
	}
	v.CreatedAt = unixToTime(created)
	return &v, nil
}

func (s *Store) CreateVote(ctx context.Context, v *Vote) error {
	if v.CreatedAt.IsZero() {
		v.CreatedAt = time.Now().UTC()
	}
	if v.Metadata == "" {
		v.Metadata = "{}"
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO votes (mode_id, mode_name, left_image_id, right_image_id,
		 winner_image_id, loser_image_id, value, min_value, max_value, weight,
		 user_id, username, metadata, duration_ms, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		v.ModeID, v.ModeName, v.LeftImageID, v.RightImageID,
		v.WinnerImageID, v.LoserImageID, v.Value, v.MinValue, v.MaxValue, v.Weight,
		v.UserID, v.Username, v.Metadata, v.DurationMs, timeToUnix(v.CreatedAt))
	if err != nil {
		return err
	}
	v.ID, _ = res.LastInsertId()
	return nil
}

func (s *Store) GetVote(ctx context.Context, id int64) (*Vote, error) {
	return scanVote(s.db.QueryRowContext(ctx, `SELECT `+voteColumns+` FROM votes WHERE id = ?`, id))
}

// UpdateVote persists every mutable column of v.
func (s *Store) UpdateVote(ctx context.Context, v *Vote) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE votes SET mode_id = ?, mode_name = ?, left_image_id = ?, right_image_id = ?,
		 winner_image_id = ?, loser_image_id = ?, value = ?, min_value = ?, max_value = ?,
		 weight = ?, metadata = ?, duration_ms = ? WHERE id = ?`,
		v.ModeID, v.ModeName, v.LeftImageID, v.RightImageID,
		v.WinnerImageID, v.LoserImageID, v.Value, v.MinValue, v.MaxValue,
		v.Weight, v.Metadata, v.DurationMs, v.ID)
	return err
}

func (s *Store) DeleteVote(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM votes WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteVotesByIDs(ctx context.Context, ids []int64) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var deleted int64
	for start := 0; start < len(ids); start += 400 {
		end := min(start+400, len(ids))
		chunk := ids[start:end]
		args := make([]any, len(chunk))
		for i, id := range chunk {
			args[i] = id
		}
		res, err := tx.ExecContext(ctx, `DELETE FROM votes WHERE id IN (`+placeholders(len(chunk))+`)`, args...)
		if err != nil {
			return 0, err
		}
		n, _ := res.RowsAffected()
		deleted += n
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return deleted, nil
}

// ListVotes returns matching votes newest first plus the total count before
// pagination. Author/TitleRegex filters need image metadata and are applied
// in Go, which keeps regex semantics identical across SQLite builds.
func (s *Store) ListVotes(ctx context.Context, f VoteFilter) ([]Vote, int, error) {
	where, args := buildVoteWhere(f)

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

	query := `SELECT ` + voteColumns + ` FROM votes` + where + ` ORDER BY created_at DESC, id DESC`
	if !complexFilter {
		var total int
		if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM votes`+where, args...).Scan(&total); err != nil {
			return nil, 0, err
		}
		if !fetchAll {
			query += ` LIMIT ? OFFSET ?`
			args = append(args, defaultLimit, max(f.Offset, 0))
		}
		votes, err := s.queryVotes(ctx, query, args)
		return votes, total, err
	}

	votes, err := s.queryVotes(ctx, query, args)
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

// getAllImagesByIDs intentionally includes soft-deleted rows for filtering.
func (s *Store) getAllImagesByIDs(ctx context.Context, ids []string) (map[string]*Image, error) {
	out := make(map[string]*Image, len(ids))
	ids = compactUnique(ids)
	if len(ids) == 0 {
		return out, nil
	}
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+imageColumns+` FROM images WHERE id IN (`+placeholders(len(ids))+`)`, args...)
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

func (s *Store) queryVotes(ctx context.Context, query string, args []any) ([]Vote, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Vote, 0)
	for rows.Next() {
		v, err := scanVote(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *v)
	}
	return out, rows.Err()
}

func buildVoteWhere(f VoteFilter) (string, []any) {
	where := ` WHERE 1=1`
	args := make([]any, 0, 6)
	if f.From != nil {
		where += ` AND created_at >= ?`
		args = append(args, *f.From)
	}
	if f.To != nil {
		where += ` AND created_at <= ?`
		args = append(args, *f.To)
	}
	if username := strings.TrimSpace(f.Username); username != "" {
		where += ` AND username = ? COLLATE NOCASE`
		args = append(args, username)
	}
	if mode := strings.TrimSpace(f.Mode); mode != "" {
		where += ` AND (mode_name = ? COLLATE NOCASE OR CAST(mode_id AS TEXT) = ?)`
		args = append(args, mode, mode)
	}
	if imageID := strings.TrimSpace(f.ImageID); imageID != "" {
		where += ` AND (left_image_id = ? OR right_image_id = ?)`
		args = append(args, imageID, imageID)
	}
	return where, args
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
