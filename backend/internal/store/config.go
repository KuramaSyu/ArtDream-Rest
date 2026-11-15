package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	configKeyRankingAnonymized      = "ranking_anonymized"
	configKeyDefaultImageVisibility = "default_image_visibility"
	configKeyRankRotationEnabled    = "rank_rotation_enabled"
	configKeyRankRotationPercent    = "rank_rotation_percent"
	configKeyExcludeAuthors         = "rank_filter_exclude_authors"
	configKeyExcludeTitles          = "rank_filter_exclude_titles"
	configKeyIncludeAuthors         = "rank_filter_include_authors"
	configKeyIncludeTitles          = "rank_filter_include_titles"
)

// GetAppConfig reads the server-side frontend configuration and the pattern
// allow/deny lists that filter public image and ranking results.
func (s *Store) GetAppConfig(ctx context.Context) (AppConfig, error) {
	config := AppConfig{
		DefaultImageVisibility: "all",
		RankRotationPercent:    60,
		ExcludeAuthors:         []string{},
		ExcludeTitles:          []string{},
		IncludeAuthors:         []string{},
		IncludeTitles:          []string{},
	}
	rows, err := s.db.QueryContext(ctx, `SELECT key, value, updated_at FROM app_config`)
	if err != nil {
		return config, err
	}
	defer rows.Close()

	var updated int64
	for rows.Next() {
		var key, value string
		var at int64
		if err := rows.Scan(&key, &value, &at); err != nil {
			return config, err
		}
		if at > updated {
			updated = at
		}
		switch key {
		case configKeyRankingAnonymized:
			config.RankingAnonymized = value == "true" || value == "1"
		case configKeyDefaultImageVisibility:
			if visibility, err := NormalizeVisibility(value); err == nil {
				config.DefaultImageVisibility = visibility
			}
		case configKeyRankRotationEnabled:
			config.RankRotationEnabled = value == "true" || value == "1"
		case configKeyRankRotationPercent:
			if percent, err := strconv.Atoi(strings.TrimSpace(value)); err == nil {
				if normalized, err := NormalizeRotationPercent(percent); err == nil {
					config.RankRotationPercent = normalized
				}
			}
		case configKeyExcludeAuthors:
			config.ExcludeAuthors = decodePatternList(value)
		case configKeyExcludeTitles:
			config.ExcludeTitles = decodePatternList(value)
		case configKeyIncludeAuthors:
			config.IncludeAuthors = decodePatternList(value)
		case configKeyIncludeTitles:
			config.IncludeTitles = decodePatternList(value)
		}
	}
	if err := rows.Err(); err != nil {
		return config, err
	}
	if updated > 0 {
		config.UpdatedAt = unixToTime(updated)
	}
	return config, nil
}

func decodePatternList(raw string) []string {
	out := []string{}
	if raw == "" {
		return out
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return []string{}
	}
	return out
}

// SetRankingAnonymized persists the global ranking-anonymization switch.
func (s *Store) SetRankingAnonymized(ctx context.Context, enabled bool) error {
	value := "false"
	if enabled {
		value = "true"
	}
	return s.setConfigValue(ctx, configKeyRankingAnonymized, value)
}

// SetDefaultImageVisibility persists the fallback public/private filter for
// image requests that do not specify their own visibility parameter.
func (s *Store) SetDefaultImageVisibility(ctx context.Context, visibility string) error {
	normalized, err := NormalizeVisibility(visibility)
	if err != nil {
		return err
	}
	return s.setConfigValue(ctx, configKeyDefaultImageVisibility, normalized)
}

// NormalizeRotationPercent validates the rank-rotation minimum share.
func NormalizeRotationPercent(percent int) (int, error) {
	if percent < 0 || percent > 100 {
		return 0, fmt.Errorf("rankRotationPercent must be between 0 and 100")
	}
	return percent, nil
}

// SetRankRotationEnabled persists the admin switch for rank-result rotation.
func (s *Store) SetRankRotationEnabled(ctx context.Context, enabled bool) error {
	value := "false"
	if enabled {
		value = "true"
	}
	return s.setConfigValue(ctx, configKeyRankRotationEnabled, value)
}

// SetRankRotationPercent persists the minimum share of a rank page that should
// differ from the previous period. Range: 0-100.
func (s *Store) SetRankRotationPercent(ctx context.Context, percent int) error {
	normalized, err := NormalizeRotationPercent(percent)
	if err != nil {
		return err
	}
	return s.setConfigValue(ctx, configKeyRankRotationPercent, strconv.Itoa(normalized))
}

// SetFilterLists persists the four regex pattern lists used to hide images
// from public API results. Entries are normalized and validated before write.
func (s *Store) SetFilterLists(ctx context.Context, cfg AppConfig) error {
	lists := []struct {
		key     string
		entries []string
	}{
		{configKeyExcludeAuthors, cfg.ExcludeAuthors},
		{configKeyExcludeTitles, cfg.ExcludeTitles},
		{configKeyIncludeAuthors, cfg.IncludeAuthors},
		{configKeyIncludeTitles, cfg.IncludeTitles},
	}
	for _, list := range lists {
		normalized, err := NormalizePatternList(list.entries)
		if err != nil {
			return err
		}
		encoded, err := json.Marshal(normalized)
		if err != nil {
			return err
		}
		if err := s.setConfigValue(ctx, list.key, string(encoded)); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) setConfigValue(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO app_config (key, value, updated_at) VALUES (?, ?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		key, value, time.Now().UTC().Unix(),
	)
	return err
}

// NormalizePatternList trims entries, drops empties/duplicates and verifies
// that every entry is a valid regular expression.
func NormalizePatternList(in []string) ([]string, error) {
	out := make([]string, 0, len(in))
	seen := make(map[string]struct{}, len(in))
	for _, raw := range in {
		pattern := trimPattern(raw)
		if pattern == "" {
			continue
		}
		if len([]rune(pattern)) > 200 {
			return nil, fmt.Errorf("pattern is longer than 200 characters: %s", pattern)
		}
		if _, err := CompilePattern(pattern); err != nil {
			return nil, fmt.Errorf("invalid regex %q: %w", pattern, err)
		}
		if _, ok := seen[pattern]; ok {
			continue
		}
		seen[pattern] = struct{}{}
		out = append(out, pattern)
	}
	return out, nil
}

func trimPattern(s string) string {
	start, end := 0, len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}
