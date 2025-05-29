package websocket

import (
	"collab-learn/internal/database"
	"collab-learn/internal/models"
	"collab-learn/internal/redis"
	"encoding/json"
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
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type WSHandler struct {
	hub   *Hub
	db    *database.DB
	redis *redis.Client
}

func NewWSHandler(hub *Hub, db *database.DB, redis *redis.Client) *WSHandler {
	return &WSHandler{
		hub:   hub,
		db:    db,
		redis: redis,
	}
}

func (h *WSHandler) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	boardID := chi.URLParam(r, "id")
	
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}

	client := &Client{
		ID:      uuid.New().String(),
		BoardID: boardID,
		Send:    make(chan []byte, 256),
		Hub:     h.hub,
	}

	h.hub.register <- client

	go client.writePump(conn)
	go client.readPump(conn, h.db, h.redis)
}

func (c *Client) readPump(conn *websocket.Conn, db *database.DB, redis *redis.Client) {
	defer func() {
		c.Hub.unregister <- c
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
			var update models.BoardUpdate
			data, _ := json.Marshal(msg.Data)
			json.Unmarshal(data, &update)

			query := `UPDATE boards SET code_html = $1, code_css = $2, updated_at = $3 WHERE id = $4`
			_, err := db.Exec(query, update.CodeHTML, update.CodeCSS, time.Now(), c.BoardID)
			if err != nil {
				log.Printf("Error updating board: %v", err)
				continue
			}

			update.BoardID = c.BoardID
			redis.PublishBoardUpdate(c.BoardID, update)

			var board models.Board
			board.ID = c.BoardID
			board.CodeHTML = update.CodeHTML
			board.CodeCSS = update.CodeCSS
			redis.CacheBoard(c.BoardID, board, 15*time.Minute)

			msg.BoardID = c.BoardID
			c.Hub.broadcast <- msg
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

			conn.WriteMessage(websocket.TextMessage, message)

		case <-ticker.C:
			conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}