package websocket

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestLegacyWebSocketWritesCannotBypassConditionalHTTPUpdates(t *testing.T) {
	hub := &Hub{broadcast: make(chan Message, 1), unregister: make(chan *Client, 1)}
	var owner *Client
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		owner = &Client{ID: "old-editor", BoardID: "board", Hub: hub}
		owner.readPump(conn)
	}))
	defer server.Close()
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := conn.WriteJSON(Message{Type: "update", Data: map[string]string{"code_html": "old write", "code_css": ""}}); err != nil {
		t.Fatal(err)
	}
	select {
	case message := <-hub.broadcast:
		if message.Type != "error" || message.Recipient != owner || message.BoardID != "board" {
			t.Fatalf("%+v", message)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("legacy writer did not receive an explicit protocol error")
	}
}
