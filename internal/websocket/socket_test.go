package websocket

import (
	"collab-learn/internal/models"
	"context"
	"encoding/json"
	"net"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

type existingBoard struct{}

func (existingBoard) GetBoard(_ context.Context, id string) (*models.Board, error) {
	return &models.Board{ID: id}, nil
}

func socketNode(t *testing.T, h *Hub, room string) *websocket.Conn {
	t.Helper()
	r := chi.NewRouter()
	r.Get("/api/boards/{id}/ws", NewWSHandler(h, existingBoard{}).HandleWebSocket)
	server := httptest.NewServer(r)
	t.Cleanup(server.Close)
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/api/boards/"+room+"/ws", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}
func socketEvent(t *testing.T, conn *websocket.Conn, match func(wireMessage) bool) {
	t.Helper()
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	for {
		var raw struct {
			Type    string          `json:"type"`
			BoardID string          `json:"board_id"`
			Data    json.RawMessage `json:"data"`
		}
		if err := conn.ReadJSON(&raw); err != nil {
			t.Fatal(err)
		}
		if match(wireMessage{raw.Type, raw.BoardID, raw.Data}) {
			return
		}
	}
}

func TestRealWebSocketsAcrossNodesAndShutdown(t *testing.T) {
	server := miniredis.RunT(t)
	ra, rb := redisClient(t, server.Addr()), redisClient(t, server.Addr())
	a, b := runningHub(t, ra), runningHub(t, rb)
	room := uuid.NewString()
	left, right := socketNode(t, a, room), socketNode(t, b, room)
	socketEvent(t, left, countAt(2))
	socketEvent(t, right, countAt(2))
	board := models.Board{ID: room, Revision: 4, CodeHTML: "through Redis and WebSocket"}
	a.BroadcastBoard(&board)
	if err := ra.PublishBoardUpdate(room, board); err != nil {
		t.Fatal(err)
	}
	socketEvent(t, left, updateAt(4))
	socketEvent(t, right, updateAt(4))
	a.Close()
	left.SetReadDeadline(time.Now().Add(3 * time.Second))
	for {
		_, _, err := left.ReadMessage()
		if err != nil {
			if timeout, ok := err.(net.Error); ok && timeout.Timeout() {
				t.Fatal("socket remained open after shutdown")
			}
			break
		}
	}
	socketEvent(t, right, countAt(1))
}
