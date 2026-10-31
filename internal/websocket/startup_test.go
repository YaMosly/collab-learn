package websocket

import (
	"collab-learn/internal/models"
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
)

func TestHubStartupContextDoesNotOwnWorkers(t *testing.T) {
	s := miniredis.RunT(t)
	c := redisClient(t, s.Addr())
	h := NewHub(c)
	defer h.Close()
	if h.Active() {
		t.Fatal("unstarted hub is active")
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := h.StartContext(canceled); err == nil || h.Active() {
		t.Fatal("canceled startup must not start workers")
	}
	startup, finish := context.WithTimeout(context.Background(), time.Second)
	if err := h.StartContext(startup); err != nil {
		finish()
		t.Fatal(err)
	}
	finish()
	client := roomClient(t, h, "startup-room")
	if err := c.PublishBoardUpdate("startup-room", &models.Board{ID: "startup-room", Revision: 1, CodeHTML: "still alive"}); err != nil {
		t.Fatal(err)
	}
	nextMessage(t, client, updateAt(1))
	if !h.Active() {
		t.Fatal("startup cancellation stopped running hub")
	}
	h.Close()
	if h.Active() {
		t.Fatal("closed hub is active")
	}
}
