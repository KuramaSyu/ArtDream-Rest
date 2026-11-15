package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// Store owns the SQLite connection and all persistence queries.
type Store struct {
	db *sql.DB
}

// Open creates (if needed) and migrates the database at path.
func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create data directory: %w", err)
	}

	dsn := path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	// SQLite serializes writes anyway; a single connection keeps the
	// pure-Go driver predictable and avoids SQLITE_BUSY storms.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	s := &Store{db: db}
	if err := s.migrate(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate(ctx context.Context) error {
	schema := `
CREATE TABLE IF NOT EXISTS users (
	id            INTEGER PRIMARY KEY AUTOINCREMENT,
	username      TEXT NOT NULL COLLATE NOCASE UNIQUE,
	display_name  TEXT NOT NULL DEFAULT '',
	password_hash TEXT NOT NULL,
	is_admin      INTEGER NOT NULL DEFAULT 0,
	created_at    INTEGER NOT NULL,
	created_by    INTEGER
);

CREATE TABLE IF NOT EXISTS sessions (
	token      TEXT PRIMARY KEY,
	user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	created_at INTEGER NOT NULL,
	expires_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS sessions_user_idx ON sessions(user_id);

CREATE TABLE IF NOT EXISTS images (
	id                TEXT PRIMARY KEY,
	author            TEXT NOT NULL,
	title             TEXT NOT NULL DEFAULT '',
	original_filename TEXT NOT NULL DEFAULT '',
	storage_path      TEXT NOT NULL,
	mime_type         TEXT NOT NULL,
	width             INTEGER NOT NULL DEFAULT 0,
	height            INTEGER NOT NULL DEFAULT 0,
	size_bytes        INTEGER NOT NULL DEFAULT 0,
	claim_token       TEXT NOT NULL DEFAULT '',
	uploaded_by       INTEGER REFERENCES users(id) ON DELETE SET NULL,
	created_at        INTEGER NOT NULL,
	deleted_at        INTEGER
);
CREATE INDEX IF NOT EXISTS images_author_idx ON images(author COLLATE NOCASE);
CREATE INDEX IF NOT EXISTS images_created_idx ON images(created_at);

CREATE TABLE IF NOT EXISTS app_config (
	key        TEXT PRIMARY KEY,
	value      TEXT NOT NULL,
	updated_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS vote_modes (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	name        TEXT NOT NULL COLLATE NOCASE UNIQUE,
	description TEXT NOT NULL DEFAULT '',
	min_value   INTEGER NOT NULL DEFAULT 1,
	max_value   INTEGER NOT NULL DEFAULT 5,
	is_binary   INTEGER NOT NULL DEFAULT 0,
	created_at  INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS votes (
	id              INTEGER PRIMARY KEY AUTOINCREMENT,
	mode_id         INTEGER NOT NULL,
	mode_name       TEXT NOT NULL,
	left_image_id   TEXT NOT NULL,
	right_image_id  TEXT NOT NULL,
	winner_image_id TEXT,
	loser_image_id  TEXT,
	value           INTEGER,
	min_value       INTEGER NOT NULL,
	max_value       INTEGER NOT NULL,
	weight          REAL NOT NULL DEFAULT 1,
	user_id         INTEGER,
	username        TEXT NOT NULL DEFAULT '',
	metadata        TEXT NOT NULL DEFAULT '{}',
	duration_ms     INTEGER,
	created_at      INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS votes_created_idx ON votes(created_at);
CREATE INDEX IF NOT EXISTS votes_user_idx ON votes(user_id);
CREATE INDEX IF NOT EXISTS votes_left_idx ON votes(left_image_id);
CREATE INDEX IF NOT EXISTS votes_right_idx ON votes(right_image_id);
CREATE INDEX IF NOT EXISTS votes_mode_idx ON votes(mode_id);
`
	if _, err := s.db.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("migrate schema: %w", err)
	}

	// Databases created before the submit page existed need the claim column.
	if err := s.ensureColumn(ctx, "images", "claim_token", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}

	now := time.Now().Unix()
	if _, err := s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO app_config (key, value, updated_at) VALUES ('ranking_anonymized', 'false', ?)`,
		now,
	); err != nil {
		return fmt.Errorf("seed frontend config: %w", err)
	}

	modes := []struct {
		name, description string
		min, max          int
		binary            bool
	}{
		{"duel", "Classic duel: one image wins, the other loses.", 0, 1, true},
		{"ahp5", "Analytic hierarchy scale from 1 (left far better) to 5 (right far better).", 1, 5, false},
		{"ahp100", "Fine-grained analytic hierarchy scale from 1 (left far better) to 100 (right far better).", 1, 100, false},
	}
	for _, m := range modes {
		if _, err := s.db.ExecContext(ctx,
			`INSERT OR IGNORE INTO vote_modes (name, description, min_value, max_value, is_binary, created_at)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			m.name, m.description, m.min, m.max, boolInt(m.binary), now,
		); err != nil {
			return fmt.Errorf("seed vote mode %s: %w", m.name, err)
		}
	}
	return nil
}

// ensureColumn adds a column to an existing table (a tiny, dependency-free
// migration helper for additive changes).
func (s *Store) ensureColumn(ctx context.Context, table, column, definition string) error {
	rows, err := s.db.QueryContext(ctx, "PRAGMA table_info("+table+")")
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, typ string
		var notNull, pk int
		var dflt any
		if err := rows.Scan(&cid, &name, &typ, &notNull, &dflt, &pk); err != nil {
			return err
		}
		if name == column {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, "ALTER TABLE "+table+" ADD COLUMN "+column+" "+definition)
	return err
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func unixToTime(v int64) time.Time { return time.Unix(v, 0).UTC() }

func timeToUnix(t time.Time) int64 { return t.UTC().Unix() }
