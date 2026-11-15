package store

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm/clause"
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
	var rows []moderationRow
	if err := s.dbWithContext(ctx).
		Order("image_id ASC, time_window ASC").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]ImageModeration, 0, len(rows))
	for i := range rows {
		out = append(out, *rows[i].toModel())
	}
	return out, nil
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
	row := moderationToRow(m)
	return s.dbWithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "image_id"}, {Name: "time_window"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"hidden", "deranked", "note", "updated_at", "updated_by",
			}),
		}).
		Create(row).Error
}

// DeleteImageModeration removes an override, falling back to the "all" rule.
func (s *Store) DeleteImageModeration(ctx context.Context, imageID, window string) error {
	window, err := NormalizeModerationWindow(window)
	if err != nil {
		return err
	}
	res := s.dbWithContext(ctx).
		Where("image_id = ? AND time_window = ?", imageID, window).
		Delete(&moderationRow{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteModerationForImage cleans up decisions when an image is deleted.
func (s *Store) DeleteModerationForImage(ctx context.Context, imageID string) error {
	return s.dbWithContext(ctx).Where("image_id = ?", imageID).Delete(&moderationRow{}).Error
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