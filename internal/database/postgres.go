package database

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	_ "github.com/lib/pq"
)

type DB struct {
	*sql.DB
}

func NewDB(host, port, user, password, dbname string) (*DB, error) {
	psqlInfo := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		host, port, user, password, dbname)

	db, err := sql.Open("postgres", psqlInfo)
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}

	return &DB{db}, nil
}

func (db *DB) Migrate() error {
	for _, migrationFile := range []string{"migrations/001_initial.sql", "migrations/002_board_revision.sql"} {
		content, err := os.ReadFile(migrationFile)
		if err != nil {
			return fmt.Errorf("read %s: %w", migrationFile, err)
		}
		if _, err := db.Exec(string(content)); err != nil {
			return fmt.Errorf("execute %s: %w", migrationFile, err)
		}
	}

	log.Println("Database migration completed successfully")
	return nil
}
