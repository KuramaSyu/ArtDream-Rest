package store

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Store owns the SQLite connection and all persistence queries.
type Store struct {
	db *gorm.DB
}

// Open creates (if needed) and migrates the database at path.
func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create data directory: %w", err)
	}

	dsn := path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	// SQLite serializes writes anyway; a single connection keeps the
	// pure-Go driver predictable and avoids SQLITE_BUSY storms.
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)

	s := &Store{db: db}
	if err := s.migrate(context.Background()); err != nil {
		sqlDB.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error {
	sqlDB, err := s.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

func (s *Store) migrate(ctx context.Context) error {
	db := s.db.WithContext(ctx)
	migrator := db.Migrator()

	// Only build tables that do not exist yet. Running AutoMigrate on an
	// existing SQLite table can rebuild it (and lose columns) when a type or
	// collation differs slightly, so existing databases get explicit,
	// additive column migrations below instead.
	for _, model := range []any{
		&userRow{},
		&sessionRow{},
		&imageRow{},
		&appConfigRow{},
		&moderationRow{},
		&voteModeRow{},
		&voteRow{},
	} {
		if !migrator.HasTable(model) {
			if err := migrator.AutoMigrate(model); err != nil {
				return fmt.Errorf("migrate schema: %w", err)
			}
		}
	}

	// Additive column migrations for databases created by earlier versions.
	additive := []struct {
		model  any
		fields []string
	}{
		{&imageRow{}, []string{"ClaimToken", "FromPublicService", "ServiceName"}},
		{&voteRow{}, []string{"AnonymousID", "VoteSpeed"}},
	}
	for _, migration := range additive {
		for _, field := range migration.fields {
			if migrator.HasColumn(migration.model, field) {
				continue
			}
			if err := migrator.AddColumn(migration.model, field); err != nil {
				return fmt.Errorf("add column %s: %w", field, err)
			}
		}
	}
	if !migrator.HasIndex(&voteRow{}, "votes_anonymous_idx") {
		if err := migrator.CreateIndex(&voteRow{}, "votes_anonymous_idx"); err != nil {
			return fmt.Errorf("create votes_anonymous_idx: %w", err)
		}
	}

	defaults := []appConfigRow{
		{Key: configKeyRankingAnonymized, Value: "false"},
		{Key: configKeyDefaultImageVisibility, Value: VisibilityAll},
		{Key: configKeyRankRotationEnabled, Value: "false"},
		{Key: configKeyRankRotationPercent, Value: "60"},
	}
	for i := range defaults {
		defaults[i].UpdatedAt = nowUnix()
	}
	if err := s.db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(&defaults).Error; err != nil {
		return fmt.Errorf("seed frontend config: %w", err)
	}

	modes := []voteModeRow{
		{Name: "duel", Description: "Classic duel: one image wins, the other loses.", MinValue: 0, MaxValue: 1, IsBinary: 1},
		{Name: "ahp5", Description: "Analytic hierarchy scale from 1 (left far better) to 5 (right far better).", MinValue: 1, MaxValue: 5},
		{Name: "ahp100", Description: "Fine-grained analytic hierarchy scale from 1 (left far better) to 100 (right far better).", MinValue: 1, MaxValue: 100},
	}
	for i := range modes {
		modes[i].CreatedAt = nowUnix()
	}
	if err := s.db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(&modes).Error; err != nil {
		return fmt.Errorf("seed vote modes: %w", err)
	}
	return nil
}
