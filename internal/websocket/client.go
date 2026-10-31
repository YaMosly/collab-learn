package websocket

import (
	"collab-learn/internal/access"
	"collab-learn/internal/models"
	"context"
	"database/sql"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 512 * 1024
	sendQueueSize  = 16
)

type WSHandler struct {
	hub          *Hub
	db           BoardReader
	publicOrigin string
}

type BoardReader interface {
	GetBoard(context.Context, string) (*models.Board, error)
}

func NewWSHandler(hub *Hub, db BoardReader, publicOrigin string) *WSHandler {
	return &WSHandler{
		hub:          hub,
		db:           db,
		publicOrigin: publicOrigin,
	}
}

func (h *WSHandler) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	if !access.OriginAllowed(r, h.publicOrigin) {
		http.Error(w, "Origin denied", http.StatusForbidden)
		return
	}
	boardID := chi.URLParam(r, "id")
	if _, err := h.db.GetBoard(r.Context(), boardID); err != nil {
		status := http.StatusServiceUnavailable
		if errors.Is(err, sql.ErrNoRows) {
			status = http.StatusNotFound
		}
		http.Error(w, "Board unavailable", status)
		return
	}

	upgrader := websocket.Upgrader{ReadBufferSize: 1024, WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool { return access.OriginAllowed(r, h.publicOrigin) }}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}

	client := &Client{
		ID:      uuid.New().String(),
		BoardID: boardID,
		Send:    make(chan []byte, sendQueueSize),
		Hub:     h.hub,
		conn:    conn,
	}

	if !h.hub.Register(client) {
		conn.Close()
		return
	}

	go client.writePump(conn)
	go client.readPump(conn)
}

func (c *Client) readPump(conn *websocket.Conn) {
	defer func() {
		c.Hub.Unregister(c)
		conn.Close()
	}()

	conn.SetReadLimit(maxMessageSize)
	conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	// This channel is server-to-client. Bound unsolicited messages from old or
	// hostile clients as well as frame size, without limiting ping/pong control.
	window, messages := time.Now(), 0
	for {
		var msg Message
		err := conn.ReadJSON(&msg)
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			break
		}
		if time.Since(window) >= time.Minute {
			window, messages = time.Now(), 0
		}
		messages++
		if messages > 10 {
			return
		}

		switch msg.Type {
		case "update":
			// All writes now pass through the conditional HTTP endpoint. An old
			// cached editor must reload instead of bypassing revision checks.
			c.Hub.Send(Message{BoardID: c.BoardID, Type: "error", Recipient: c,
				Data: map[string]string{"message": "Reload the editor: writes require HTTP PUT with base_revision"}})
		}
	}
}

func (c *Client) writePump(conn *websocket.Conn) {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.Send:
			conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			if err := conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}

		case <-ticker.C:
			conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
