package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// Moderation windows mirror the ranking period selector. "all" is the global
// default and also the fallback for custom ranges.
const (
	ModerationAll   = "all"
	ModerationDay   = "day"
	ModerationWeek  = "week"
	ModerationMonth = "month"
	ModerationYear  = "year"
)

var moderationWindows = []string{ModerationAll, ModerationDay, ModerationWeek, ModerationMonth, ModerationYear}

// ImageModeration is an admin decision for one image in one ranking period.
// Hidden entries are omitted from public rankings; deranked entries stay
// visible but are sorted below regular entries. Neither deletes the image.
type ImageModeration struct {
	ImageID    string    `json:"imageId"`
	TimeWindow string    `json:"timeWindow"`
	Hidden     bool      `json:"hidden"`
	Deranked   bool      `json:"deranked"`
	Note       string    `json:"note"`
	UpdatedAt  time.Time `json:"updatedAt"`
	UpdatedBy  *int64    `json:"updatedBy,omitempty"`
}

// NormalizeModerationWindow validates a ranking-period key.
func NormalizeModerationWindow(window string) (string, error) {
	window = strings.ToLower(strings.TrimSpace(window))
	if window == "" {
		return ModerationAll, nil
	}
	for _, candidate := range moderationWindows {
		if window == candidate {
			return window, nil
		}
	}
	return "", fmt.Errorf("timeWindow must be one of all, day, week, month, year")
}

// ModerationWindowForLabel maps a ranking range label to a moderation window.
// Custom ranges intentionally follow the global "all" rules.
func ModerationWindowForLabel(label string) string {
	switch strings.ToLower(strings.TrimSpace(label)) {
	case ModerationDay:
		return ModerationDay
	case ModerationWeek:
		return ModerationWeek
	case ModerationMonth:
		return ModerationMonth
	case ModerationYear:
		return ModerationYear
	default:
		return ModerationAll
	}
}

// ListImageModeration returns every moderation row (small table).
func (s *Store) ListImageModeration(ctx context.Context) ([]ImageModeration, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT image_id, time_window, hidden, deranked, note, updated_at, updated_by
		 FROM image_moderation ORDER BY image_id ASC, time_window ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]ImageModeration, 0)
	for rows.Next() {
		var m ImageModeration
		var hidden, deranked int
		var updated int64
		var updatedBy sql.NullInt64
		if err := rows.Scan(&m.ImageID, &m.TimeWindow, &hidden, &deranked, &m.Note, &updated, &updatedBy); err != nil {
			return nil, err
		}
		m.Hidden = hidden != 0
		m.Deranked = deranked != 0
		m.UpdatedAt = unixToTime(updated)
		if updatedBy.Valid {
			id := updatedBy.Int64
			m.UpdatedBy = &id
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// UpsertImageModeration writes (or updates) one image/window decision.
func (s *Store) UpsertImageModeration(ctx context.Context, m *ImageModeration) error {
	window, err := NormalizeModerationWindow(m.TimeWindow)
	if err != nil {
		return err
	}
	m.TimeWindow = window
	if m.UpdatedAt.IsZero() {
		m.UpdatedAt = time.Now().UTC()
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO image_moderation (image_id, time_window, hidden, deranked, note, updated_at, updated_by)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(image_id, time_window) DO UPDATE SET
		   hidden = excluded.hidden,
		   deranked = excluded.deranked,
		   note = excluded.note,
		   updated_at = excluded.updated_at,
		   updated_by = excluded.updated_by`,
		m.ImageID, m.TimeWindow, boolInt(m.Hidden), boolInt(m.Deranked), m.Note,
		timeToUnix(m.UpdatedAt), m.UpdatedBy,
	)
	return err
}

// DeleteImageModeration removes an override, falling back to the "all" rule.
func (s *Store) DeleteImageModeration(ctx context.Context, imageID, window string) error {
	window, err := NormalizeModerationWindow(window)
	if err != nil {
		return err
	}
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM image_moderation WHERE image_id = ? AND time_window = ?`, imageID, window)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteModerationForImage cleans up decisions when an image is deleted.
func (s *Store) DeleteModerationForImage(ctx context.Context, imageID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM image_moderation WHERE image_id = ?`, imageID)
	return err
}

// ModerationFor resolves the effective decision: the exact window wins, and
// otherwise the global "all" row applies.
func ModerationFor(rows []ImageModeration, imageID, window string) (ImageModeration, bool) {
	if window == "" {
		window = ModerationAll
	}
	var fallback *ImageModeration
	for i := range rows {
		row := &rows[i]
		if row.ImageID != imageID {
			continue
		}
		if row.TimeWindow == window {
			return *row, true
		}
		if window != ModerationAll && row.TimeWindow == ModerationAll {
			fallback = row
		}
	}
	if fallback != nil {
		return *fallback, true
	}
	return ImageModeration{}, false
}
