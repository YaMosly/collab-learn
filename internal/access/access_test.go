package access

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

func TestOriginPolicy(t *testing.T) {
	for _, tc := range []struct {
		name, origin, public, site string
		tls                        bool
		status                     int
	}{
		{"native", "", "", "", false, 204}, {"same", "http://example.org", "", "same-origin", false, 204},
		{"foreign", "https://evil.org", "", "", false, 403}, {"null", "null", "", "", false, 403},
		{"port differs", "http://example.org:81", "", "", false, 403}, {"scheme differs", "https://example.org", "", "", false, 403},
		{"TLS", "https://example.org", "", "", true, 204}, {"proxy", "https://public.org", "https://public.org", "same-origin", false, 204},
		{"proxy wrong", "http://example.org", "https://public.org", "", false, 403}, {"fetch metadata", "", "", "cross-site", false, 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("PUT", "http://example.org/api/boards/id", nil)
			if tc.origin != "" {
				r.Header.Set("Origin", tc.origin)
			}
			r.Header.Set("Sec-Fetch-Site", tc.site)
			r.Header.Set("X-Forwarded-Host", "evil.org")
			r.Header.Set("X-Forwarded-Proto", "https")
			if tc.tls {
				r.TLS = &tls.ConnectionState{}
			}
			w := httptest.NewRecorder()
			SameOrigin(tc.public)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })).ServeHTTP(w, r)
			if w.Code != tc.status {
				t.Fatalf("got %d want %d", w.Code, tc.status)
			}
		})
	}
	r := httptest.NewRequest("GET", "http://example.org", nil)
	r.Header.Add("Origin", "http://example.org")
	r.Header.Add("Origin", "https://evil.org")
	if OriginAllowed(r, "") {
		t.Fatal("duplicate origins accepted")
	}
}

func TestPublicOriginValidation(t *testing.T) {
	for _, value := range []string{"https://example.org/", "https://user:secret@example.org", "null", "*", "example.org", "https://example.org?q=1", "https://example.org#x"} {
		if err := ValidateOrigin(value); err == nil || strings.Contains(err.Error(), "secret") {
			t.Fatalf("invalid origin accepted or echoed: %v", err)
		}
	}
	for _, value := range []string{"", "https://example.org", "http://localhost:8080", "http://[::1]:8080"} {
		if err := ValidateOrigin(value); err != nil {
			t.Fatal(err)
		}
	}
}

type counterFunc func(context.Context, string, int, time.Duration) (bool, time.Duration, error)

func (f counterFunc) Take(ctx context.Context, key string, n int, d time.Duration) (bool, time.Duration, error) {
	return f(ctx, key, n, d)
}

func TestLimiterUsesPeerAndFailsClosed(t *testing.T) {
	var keys []string
	calls := 0
	counter := counterFunc(func(ctx context.Context, key string, n int, d time.Duration) (bool, time.Duration, error) {
		if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) > time.Second {
			t.Fatal("unbounded counter call")
		}
		if n != 30 || d != time.Minute {
			t.Fatal("wrong policy")
		}
		keys = append(keys, key)
		switch len(keys) {
		case 1:
			return true, 0, nil
		case 2:
			return false, 1500 * time.Millisecond, nil
		default:
			return false, 0, errors.New("private redis detail")
		}
	})
	handler := Limit(counter, Create)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(201) }))
	for i, expected := range []int{201, 429, 503} {
		r := httptest.NewRequest("POST", "/api/boards", nil)
		r.RemoteAddr = "192.0.2.1:1234"
		r.Header.Set("X-Forwarded-For", []string{"1.1.1.1", "2.2.2.2", "3.3.3.3"}[i])
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != expected || strings.Contains(w.Body.String(), "private") {
			t.Fatalf("status=%d body=%s", w.Code, w.Body)
		}
		if i > 0 && w.Header().Get("Retry-After") != "2" {
			t.Fatal("missing rounded retry window")
		}
	}
	if calls != 1 || keys[0] != keys[1] || keys[1] != keys[2] || strings.Contains(keys[0], "192.0.2.1") {
		t.Fatal("forwarded header bypass or raw IP retention")
	}
}

func TestAccessLogDoesNotExposeLinks(t *testing.T) {
	var output bytes.Buffer
	old := log.Writer()
	log.SetOutput(&output)
	defer log.SetOutput(old)
	router := chi.NewRouter()
	router.Use(Headers)
	router.Use(Logger)
	router.Get("/api/boards/{id}", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/api/boards/bearer-secret?board=another-secret", nil))
	if strings.Contains(output.String(), "secret") || !strings.Contains(output.String(), "/api/boards/{id}") {
		t.Fatalf("unsafe log: %s", output.String())
	}
	if w.Header().Get("Referrer-Policy") != "no-referrer" || w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("missing browser protection headers")
	}
}
