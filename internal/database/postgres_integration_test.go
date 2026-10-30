package database

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
)

// Opt-in: a fresh schema is created and removed; no existing board is touched.
func TestPostgresConcurrentRevisions(t *testing.T) {
	dsn := os.Getenv("COLLABLEARN_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set COLLABLEARN_TEST_DATABASE_URL to an isolated PostgreSQL URL")
	}
	parsed, err := url.Parse(dsn)
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") {
		t.Fatal("expected a PostgreSQL URL")
	}
	admin, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	schema := "collablearn_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := admin.Exec("CREATE SCHEMA " + schema); err != nil {
		t.Fatal(err)
	}
	defer admin.Exec("DROP SCHEMA " + schema + " CASCADE")
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	pool, err := sql.Open("postgres", parsed.String())
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	db := &DB{pool}
	for _, file := range []string{"001_initial.sql", "002_board_revision.sql", "002_board_revision.sql"} {
		migration, err := os.ReadFile(filepath.Join("..", "..", "migrations", file))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(string(migration)); err != nil {
			t.Fatal(err)
		}
	}
	id := uuid.NewString()
	ctx := context.Background()
	if _, err := db.CreateBoard(ctx, id); err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, text := range []string{"A", "B"} {
		wg.Add(1)
		go func(value string) {
			defer wg.Done()
			_, err := db.SaveBoard(ctx, id, 0, value, "")
			results <- err
		}(text)
	}
	wg.Wait()
	close(results)
	saved, conflicts := 0, 0
	for err := range results {
		if err == nil {
			saved++
		} else if errors.Is(err, ErrConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	current, err := db.GetBoard(ctx, id)
	if err != nil || saved != 1 || conflicts != 1 || current.Revision != 1 {
		t.Fatalf("saved=%d conflicts=%d board=%+v err=%v", saved, conflicts, current, err)
	}
}
