package store

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
)

// ErrNotFound is returned when a row cannot be found.
var ErrNotFound = errors.New("not found")

// ErrConflict is returned for uniqueness violations.
var ErrConflict = errors.New("conflict")

func (s *Store) dbWithContext(ctx context.Context) *gorm.DB {
	return s.db.WithContext(ctx)
}

// CountUsers returns the total number of accounts. Used to gate bootstrap.
func (s *Store) CountUsers(ctx context.Context) (int, error) {
	var n int64
	if err := s.dbWithContext(ctx).Model(&userRow{}).Count(&n).Error; err != nil {
		return 0, err
	}
	return int(n), nil
}

// CreateUser inserts an account and fills u.ID / u.CreatedAt.
func (s *Store) CreateUser(ctx context.Context, u *User) error {
	if u.DisplayName == "" {
		u.DisplayName = u.Username
	}
	if u.CreatedAt.IsZero() {
		u.CreatedAt = time.Now().UTC()
	}
	row := userToRow(u)
	row.ID = 0
	if err := s.dbWithContext(ctx).Create(row).Error; err != nil {
		if isUniqueViolation(err) {
			return ErrConflict
		}
		return err
	}
	u.ID = row.ID
	return nil
}

func (s *Store) GetUserByUsername(ctx context.Context, username string) (*User, error) {
	var row userRow
	err := s.dbWithContext(ctx).
		Where("username = ? COLLATE NOCASE", username).
		Take(&row).Error
	if err != nil {
		return nil, translateError(err)
	}
	return row.toModel(), nil
}

func (s *Store) GetUserByID(ctx context.Context, id int64) (*User, error) {
	var row userRow
	if err := s.dbWithContext(ctx).First(&row, "id = ?", id).Error; err != nil {
		return nil, translateError(err)
	}
	return row.toModel(), nil
}

func (s *Store) ListUsers(ctx context.Context, f UserFilter) ([]User, int, error) {
	var total int64
	if err := s.dbWithContext(ctx).Model(&userRow{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	limit := f.Limit
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	var rows []userRow
	if err := s.dbWithContext(ctx).
		Order("is_admin DESC, username ASC").
		Limit(limit).
		Offset(max(f.Offset, 0)).
		Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	out := make([]User, 0, len(rows))
	for i := range rows {
		out = append(out, *rows[i].toModel())
	}
	return out, int(total), nil
}

// UpdateUser changes display name, password hash and/or the admin flag. Nil
// arguments leave the respective column untouched.
func (s *Store) UpdateUser(ctx context.Context, id int64, displayName, passwordHash *string, isAdmin *bool) error {
	updates := map[string]any{}
	if displayName != nil {
		updates["display_name"] = *displayName
	}
	if passwordHash != nil {
		updates["password_hash"] = *passwordHash
	}
	if isAdmin != nil {
		updates["is_admin"] = boolInt64(*isAdmin)
	}
	if len(updates) == 0 {
		return nil
	}
	return s.dbWithContext(ctx).Model(&userRow{}).Where("id = ?", id).Updates(updates).Error
}

func (s *Store) DeleteUser(ctx context.Context, id int64) error {
	return s.dbWithContext(ctx).Where("id = ?", id).Delete(&userRow{}).Error
}

// --- sessions -------------------------------------------------------------

func (s *Store) CreateSession(ctx context.Context, token string, userID int64, ttl time.Duration) error {
	now := time.Now().UTC()
	row := sessionRow{
		Token:     token,
		UserID:    userID,
		CreatedAt: unixValue(now),
		ExpiresAt: unixValue(now.Add(ttl)),
	}
	return s.dbWithContext(ctx).Create(&row).Error
}

func (s *Store) GetSessionUser(ctx context.Context, token string) (*User, error) {
	var row userRow
	err := s.dbWithContext(ctx).
		Table("sessions AS s").
		Select("u.id, u.username, u.display_name, u.password_hash, u.is_admin, u.created_at, u.created_by").
		Joins("JOIN users u ON u.id = s.user_id").
		Where("s.token = ? AND s.expires_at > ?", token, nowUnix()).
		Take(&row).Error
	if err != nil {
		return nil, translateError(err)
	}
	return row.toModel(), nil
}

func (s *Store) DeleteSession(ctx context.Context, token string) error {
	return s.dbWithContext(ctx).Where("token = ?", token).Delete(&sessionRow{}).Error
}

func (s *Store) DeleteExpiredSessions(ctx context.Context) error {
	return s.dbWithContext(ctx).Where("expires_at <= ?", nowUnix()).Delete(&sessionRow{}).Error
}

func translateError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	if isUniqueViolation(err) {
		return ErrConflict
	}
	return err
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "UNIQUE constraint failed") || strings.Contains(msg, "constraint failed: UNIQUE")
}

func nowUnix() int64 { return time.Now().UTC().Unix() }

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