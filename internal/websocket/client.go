package websocket

import (
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

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type WSHandler struct {
	hub *Hub
	db  BoardReader
}

type BoardReader interface {
	GetBoard(context.Context, string) (*models.Board, error)
}

func NewWSHandler(hub *Hub, db BoardReader) *WSHandler {
	return &WSHandler{
		hub: hub,
		db:  db,
	}
}

func (h *WSHandler) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	boardID := chi.URLParam(r, "id")
	if _, err := h.db.GetBoard(r.Context(), boardID); err != nil {
		status := http.StatusServiceUnavailable
		if errors.Is(err, sql.ErrNoRows) {
			status = http.StatusNotFound
		}
		http.Error(w, "Board unavailable", status)
		return
	}

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

	for {
		var msg Message
		err := conn.ReadJSON(&msg)
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			break
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
