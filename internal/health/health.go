package health

import (
	"context"
	"encoding/json"
	"net/http"
	"sync/atomic"
	"time"
)

type Handler struct {
	Database, Redis func(context.Context) error
	Hub             func() bool
	Stopping        atomic.Bool
}

func (h *Handler) Live(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"alive": true})
}

func (h *Handler) Ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	checks := map[string]bool{"database": h.Database(ctx) == nil, "redis": h.Redis(ctx) == nil, "hub": h.Hub(), "accepting": !h.Stopping.Load()}
	ready := true
	for _, ok := range checks {
		ready = ready && ok
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	if !ready {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	json.NewEncoder(w).Encode(checks)
}
