package health

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestReadinessAndLiveness(t *testing.T) {
	ok := func(context.Context) error { return nil }
	fail := func(context.Context) error { return errors.New("private connection detail") }
	for _, tc := range []struct {
		name          string
		db, redis     func(context.Context) error
		hub, stopping bool
		status        int
	}{
		{"ready", ok, ok, true, false, 200}, {"database down", fail, ok, true, false, 503},
		{"redis down", ok, fail, true, false, 503}, {"hub closed", ok, ok, false, false, 503}, {"shutdown", ok, ok, true, true, 503},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := &Handler{Database: tc.db, Redis: tc.redis, Hub: func() bool { return tc.hub }}
			h.Stopping.Store(tc.stopping)
			w := httptest.NewRecorder()
			h.Ready(w, httptest.NewRequest("GET", "/readyz", nil))
			if w.Code != tc.status || strings.Contains(w.Body.String(), "private") || w.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("readiness: %d %s", w.Code, w.Body)
			}
			w = httptest.NewRecorder()
			h.Live(w, httptest.NewRequest("GET", "/livez", nil))
			if w.Code != 200 {
				t.Fatal("liveness must not depend on external services")
			}
		})
	}
}

func TestProbePropagatesCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	check := func(ctx context.Context) error {
		if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) > 2*time.Second {
			t.Error("probe missing bounded deadline")
		}
		return ctx.Err()
	}
	h := &Handler{Database: check, Redis: check, Hub: func() bool { return true }}
	w := httptest.NewRecorder()
	h.Ready(w, httptest.NewRequest("GET", "/readyz", nil).WithContext(ctx))
	if w.Code != 503 {
		t.Fatal("canceled probe should not be ready")
	}
}
