package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// ErrNotFound is returned when a row cannot be found.
var ErrNotFound = errors.New("not found")

// ErrConflict is returned for uniqueness violations.
var ErrConflict = errors.New("conflict")

// CountUsers returns the total number of accounts. Used to gate bootstrap.
func (s *Store) CountUsers(ctx context.Context) (int, error) {
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

// CreateUser inserts an account and fills u.ID / u.CreatedAt.
func (s *Store) CreateUser(ctx context.Context, u *User) error {
	if u.DisplayName == "" {
		u.DisplayName = u.Username
	}
	if u.CreatedAt.IsZero() {
		u.CreatedAt = time.Now().UTC()
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO users (username, display_name, password_hash, is_admin, created_at, created_by)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		u.Username, u.DisplayName, u.PasswordHash, boolInt(u.IsAdmin), timeToUnix(u.CreatedAt), u.CreatedBy,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrConflict
		}
		return err
	}
	u.ID, _ = res.LastInsertId()
	return nil
}

const userColumns = `id, username, display_name, password_hash, is_admin, created_at, created_by`

func scanUser(row interface{ Scan(...any) error }) (*User, error) {
	var u User
	var created int64
	var isAdmin int
	var createdBy sql.NullInt64
	if err := row.Scan(&u.ID, &u.Username, &u.DisplayName, &u.PasswordHash, &isAdmin, &created, &createdBy); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	u.IsAdmin = isAdmin != 0
	u.CreatedAt = unixToTime(created)
	if createdBy.Valid {
		v := createdBy.Int64
		u.CreatedBy = &v
	}
	return &u, nil
}

func (s *Store) GetUserByUsername(ctx context.Context, username string) (*User, error) {
	return scanUser(s.db.QueryRowContext(ctx, `SELECT `+userColumns+` FROM users WHERE username = ?`, username))
}

func (s *Store) GetUserByID(ctx context.Context, id int64) (*User, error) {
	return scanUser(s.db.QueryRowContext(ctx, `SELECT `+userColumns+` FROM users WHERE id = ?`, id))
}

func (s *Store) ListUsers(ctx context.Context, f UserFilter) ([]User, int, error) {
	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&total); err != nil {
		return nil, 0, err
	}
	limit := f.Limit
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+userColumns+` FROM users ORDER BY is_admin DESC, username ASC LIMIT ? OFFSET ?`,
		limit, max(f.Offset, 0))
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := make([]User, 0)
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *u)
	}
	return out, total, rows.Err()
}

// UpdateUser changes display name, password hash and/or the admin flag. Nil
// arguments leave the respective column untouched.
func (s *Store) UpdateUser(ctx context.Context, id int64, displayName, passwordHash *string, isAdmin *bool) error {
	if displayName != nil {
		if _, err := s.db.ExecContext(ctx, `UPDATE users SET display_name = ? WHERE id = ?`, *displayName, id); err != nil {
			return err
		}
	}
	if passwordHash != nil {
		if _, err := s.db.ExecContext(ctx, `UPDATE users SET password_hash = ? WHERE id = ?`, *passwordHash, id); err != nil {
			return err
		}
	}
	if isAdmin != nil {
		if _, err := s.db.ExecContext(ctx, `UPDATE users SET is_admin = ? WHERE id = ?`, boolInt(*isAdmin), id); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) DeleteUser(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, id)
	return err
}

// --- sessions -------------------------------------------------------------

func (s *Store) CreateSession(ctx context.Context, token string, userID int64, ttl time.Duration) error {
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO sessions (token, user_id, created_at, expires_at) VALUES (?, ?, ?, ?)`,
		token, userID, timeToUnix(now), timeToUnix(now.Add(ttl)))
	return err
}

func (s *Store) GetSessionUser(ctx context.Context, token string) (*User, error) {
	u, err := scanUser(s.db.QueryRowContext(ctx,
		`SELECT u.id, u.username, u.display_name, u.password_hash, u.is_admin, u.created_at, u.created_by
		 FROM sessions s JOIN users u ON u.id = s.user_id
		 WHERE s.token = ? AND s.expires_at > ?`, token, time.Now().Unix()))
	if errors.Is(err, ErrNotFound) {
		return nil, ErrNotFound
	}
	return u, err
}

func (s *Store) DeleteSession(ctx context.Context, token string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE token = ?`, token)
	return err
}

func (s *Store) DeleteExpiredSessions(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at <= ?`, time.Now().Unix())
	return err
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "UNIQUE constraint failed") || strings.Contains(msg, "constraint failed: UNIQUE")
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
