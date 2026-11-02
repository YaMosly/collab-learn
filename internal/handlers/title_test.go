package handlers

import (
	"collab-learn/internal/models"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func rename(m *memoryBoards, id, body string) *httptest.ResponseRecorder {
	r := httptest.NewRecorder()
	router(m).ServeHTTP(r, httptest.NewRequest(http.MethodPatch, "/api/boards/"+id+"/title", strings.NewReader(body)))
	return r
}

func TestRenameValidatesBeforeWriting(t *testing.T) {
	for _, body := range []string{`{}`, `{"title":null,"base_title_revision":0}`, `{"title":"x","base_title_revision":-1}`,
		`{"title":"x","base_title_revision":0,"code_html":"oops"}`, `{"title":"x","base_title_revision":0} {}`,
		`{"title":"a\nb","base_title_revision":0}`, `{"title":"` + strings.Repeat("界", 101) + `","base_title_revision":0}`,
		`{"title":"` + strings.Repeat("x", 5000) + `","base_title_revision":0}`} {
		m := &memoryBoards{board: models.Board{ID: "id"}}
		r := rename(m, "id", body)
		if r.Code != 400 || m.writes != 0 {
			t.Fatalf("status=%d writes=%d", r.Code, m.writes)
		}
	}
}

func TestRenamePreservesCodeRevisionAndAcceptsUnicodeOrEmpty(t *testing.T) {
	m := &memoryBoards{board: models.Board{ID: "id", Revision: 7, CodeHTML: "draft base", CodeCSS: "body{}"}}
	for i, title := range []string{"  <img src=x onerror=alert(1)> 🎉  ", strings.Repeat("🎉", 100), "  "} {
		body, _ := json.Marshal(map[string]interface{}{"title": title, "base_title_revision": i})
		r := rename(m, "id", string(body))
		if r.Code != 200 || m.board.Revision != 7 || m.board.CodeHTML != "draft base" || m.board.CodeCSS != "body{}" || m.board.Title != strings.TrimSpace(title) || m.board.TitleRevision != int64(i+1) {
			t.Fatal(r.Code, m.board)
		}
		if r.Header().Get("Cache-Control") != "no-store" {
			t.Fatal(r.Header())
		}
	}
}

func TestConcurrentRenamesRequireAnExplicitRetry(t *testing.T) {
	m := &memoryBoards{board: models.Board{ID: "id", Revision: 9}}
	statuses := make(chan int, 2)
	var wg sync.WaitGroup
	for _, title := range []string{"A", "B"} {
		wg.Add(1)
		go func(title string) {
			defer wg.Done()
			statuses <- rename(m, "id", `{"title":"`+title+`","base_title_revision":0}`).Code
		}(title)
	}
	wg.Wait()
	close(statuses)
	counts := map[int]int{}
	for status := range statuses {
		counts[status]++
	}
	if counts[200] != 1 || counts[409] != 1 || m.writes != 1 || m.board.Revision != 9 {
		t.Fatal(counts, m.board)
	}
}

func TestRenameMissingBoardAndStorageFailure(t *testing.T) {
	m := &memoryBoards{board: models.Board{ID: "id"}}
	if r := rename(m, "missing", `{"title":"x","base_title_revision":0}`); r.Code != 404 {
		t.Fatal(r.Code)
	}
	m.failure = errors.New("offline")
	if r := rename(m, "id", `{"title":"x","base_title_revision":0}`); r.Code != 500 {
		t.Fatal(r.Code)
	}
}
