package database

import (
	"collab-learn/migrations"
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/lib/pq"
)

type DB struct {
	*sql.DB
}

func Open(ctx context.Context, databaseURL string) (*DB, error) {
	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, err
	}

	return &DB{db}, nil
}

func (db *DB) Migrate() error {
	return db.MigrateContext(context.Background())
}

func (db *DB) MigrateContext(ctx context.Context) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// Serialize startup migrations across app instances; released on commit/rollback.
	if _, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(724061981)"); err != nil {
		return err
	}
	for _, migrationFile := range []string{"001_initial.sql", "002_board_revision.sql"} {
		content, err := migrations.Files.ReadFile(migrationFile)
		if err != nil {
			return fmt.Errorf("read %s: %w", migrationFile, err)
		}
		if _, err := tx.ExecContext(ctx, string(content)); err != nil {
			return fmt.Errorf("execute %s: %w", migrationFile, err)
		}
	}

	return tx.Commit()
}
