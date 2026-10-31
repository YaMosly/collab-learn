package handlers

import (
	"collab-learn/internal/database"
	"collab-learn/internal/models"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"
)

type memoryBoards struct {
	mu      sync.Mutex
	board   models.Board
	writes  int
	failure error
}

func (m *memoryBoards) GetBoard(_ context.Context, id string) (*models.Board, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failure != nil {
		return nil, m.failure
	}
	if id != m.board.ID {
		return nil, sql.ErrNoRows
	}
	copy := m.board
	return &copy, nil
}
func (m *memoryBoards) CreateBoard(ctx context.Context, id string) (*models.Board, error) {
	return m.GetBoard(ctx, id)
}
func (m *memoryBoards) ListBoards(context.Context) ([]models.Board, error) {
	return []models.Board{}, nil
}
func (m *memoryBoards) SaveBoard(_ context.Context, id string, revision int64, html, css string) (*models.Board, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failure != nil {
		return nil, m.failure
	}
	if id != m.board.ID {
		return nil, sql.ErrNoRows
	}
	copy := m.board
	if revision != copy.Revision {
		return &copy, database.ErrConflict
	}
	m.writes++
	m.board.Revision++
	m.board.CodeHTML = html
	m.board.CodeCSS = css
	copy = m.board
	return &copy, nil
}
func router(m *memoryBoards) http.Handler {
	h := NewBoardHandler(m, nil, nil)
	r := chi.NewRouter()
	r.Get("/api/boards/{id}", h.GetBoard)
	r.Put("/api/boards/{id}", h.UpdateBoard)
	return r
}

func TestWritesRequireExplicitRevisionAndBothFields(t *testing.T) {
	for _, body := range []string{`{}`, `{"code_html":"", "code_css":""}`, `{"base_revision":0,"code_html":null,"code_css":""}`,
		`{"base_revision":-1,"code_html":"","code_css":""}`, `{"base_revision":0,"code_html":"","code_css":""} {}`} {
		m := &memoryBoards{board: models.Board{ID: "id"}}
		rec := httptest.NewRecorder()
		router(m).ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/api/boards/id", strings.NewReader(body)))
		if rec.Code != http.StatusBadRequest || m.writes != 0 {
			t.Fatalf("%s: status %d writes %d", body, rec.Code, m.writes)
		}
	}
}

func TestEmptyDocumentIsSavedAndPublishedOnlyAfterSuccess(t *testing.T) {
	m := &memoryBoards{board: models.Board{ID: "id", CodeHTML: "delete this"}}
	published := 0
	h := NewBoardHandler(m, nil, func(b *models.Board) {
		published++
		if b.Revision != 1 {
			t.Fatal(b)
		}
	})
	r := chi.NewRouter()
	r.Put("/api/boards/{id}", h.UpdateBoard)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/api/boards/id", strings.NewReader(`{"base_revision":0,"code_html":"","code_css":""}`)))
	if rec.Code != 200 || m.board.CodeHTML != "" || published != 1 {
		t.Fatalf("status %d board %+v published %d", rec.Code, m.board, published)
	}
}

func TestStaleWriteReturns409AndCurrentDocument(t *testing.T) {
	m := &memoryBoards{board: models.Board{ID: "id", Revision: 2, CodeHTML: "current"}}
	rec := httptest.NewRecorder()
	router(m).ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/api/boards/id", strings.NewReader(`{"base_revision":1,"code_html":"old","code_css":""}`)))
	var current models.Board
	if err := json.NewDecoder(rec.Body).Decode(&current); err != nil {
		t.Fatal(err)
	}
	if rec.Code != 409 || current.CodeHTML != "current" || m.writes != 0 {
		t.Fatal(rec.Code, current, m.writes)
	}
}

func TestConcurrentHTTPWritersCannotBothSucceedFromTheSameRevision(t *testing.T) {
	m := &memoryBoards{board: models.Board{ID: "id"}}
	r := router(m)
	statuses := make(chan int, 2)
	var wg sync.WaitGroup
	for _, html := range []string{"A", "B"} {
		wg.Add(1)
		go func(value string) {
			defer wg.Done()
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/api/boards/id", strings.NewReader(`{"base_revision":0,"code_html":"`+value+`","code_css":""}`)))
			statuses <- rec.Code
		}(html)
	}
	wg.Wait()
	close(statuses)
	counts := map[int]int{}
	for status := range statuses {
		counts[status]++
	}
	if counts[200] != 1 || counts[409] != 1 || m.writes != 1 {
		t.Fatal(counts, m.writes)
	}
}

func TestMissingBoardsAndStorageErrorsHaveDifferentStatuses(t *testing.T) {
	for _, failure := range []error{nil, errors.New("offline")} {
		m := &memoryBoards{board: models.Board{ID: "id"}, failure: failure}
		rec := httptest.NewRecorder()
		router(m).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/boards/missing", nil))
		want := 404
		if failure != nil {
			want = 500
		}
		if rec.Code != want {
			t.Fatal(rec.Code, want)
		}
	}
}

func TestSnapshotCannotBeServedFromAnHTTPCache(t *testing.T) {
	m := &memoryBoards{board: models.Board{ID: "id"}}
	rec := httptest.NewRecorder()
	router(m).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/boards/id", nil))
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatal(rec.Header())
	}
}

func TestOversizedDocumentIsRejectedBeforeStorage(t *testing.T) {
	m := &memoryBoards{board: models.Board{ID: "id"}}
	rec := httptest.NewRecorder()
	body := `{"base_revision":0,"code_html":"` + strings.Repeat("x", 512*1024) + `","code_css":""}`
	router(m).ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/api/boards/id", strings.NewReader(body)))
	if rec.Code != http.StatusRequestEntityTooLarge || m.writes != 0 {
		t.Fatal(rec.Code, m.writes)
	}
}
