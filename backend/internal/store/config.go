package store

import (
	"context"
	"time"
)

// GetAppConfig reads the server-side frontend configuration.
func (s *Store) GetAppConfig(ctx context.Context) (AppConfig, error) {
	config := AppConfig{RankingAnonymized: false}
	var raw string
	var updated int64
	if err := s.db.QueryRowContext(ctx,
		`SELECT value, updated_at FROM app_config WHERE key = 'ranking_anonymized'`,
	).Scan(&raw, &updated); err != nil {
		return config, err
	}
	config.RankingAnonymized = raw == "true" || raw == "1"
	config.UpdatedAt = unixToTime(updated)
	return config, nil
}

// SetRankingAnonymized persists the global ranking-anonymization switch.
func (s *Store) SetRankingAnonymized(ctx context.Context, enabled bool) error {
	value := "false"
	if enabled {
		value = "true"
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO app_config (key, value, updated_at) VALUES ('ranking_anonymized', ?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		value, time.Now().UTC().Unix(),
	)
	return err
}
