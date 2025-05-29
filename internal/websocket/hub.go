package websocket

import (
	"collab-learn/internal/models"
	"collab-learn/internal/redis"
	"encoding/json"
	"log"
	"sync"
)

type Hub struct {
	boards     map[string]map[*Client]bool
	broadcast  chan Message
	register   chan *Client
	unregister chan *Client
	redis      *redis.Client
	mu         sync.RWMutex
}

type Message struct {
	BoardID string      `json:"board_id"`
	Type    string      `json:"type"`
	Data    interface{} `json:"data"`
}

type Client struct {
	ID      string
	BoardID string
	Send    chan []byte
	Hub     *Hub
}

func NewHub(redisClient *redis.Client) *Hub {
	return &Hub{
		boards:     make(map[string]map[*Client]bool),
		broadcast:  make(chan Message, 256),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		redis:      redisClient,
	}
}

func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			if h.boards[client.BoardID] == nil {
				h.boards[client.BoardID] = make(map[*Client]bool)
			}
			h.boards[client.BoardID][client] = true
			h.mu.Unlock()

			count, _ := h.redis.IncrementConnections(client.BoardID)
			h.broadcastConnectionCount(client.BoardID, count)
			log.Printf("Client %s connected to board %s", client.ID, client.BoardID)

		case client := <-h.unregister:
			h.mu.Lock()
			if clients, ok := h.boards[client.BoardID]; ok {
				if _, ok := clients[client]; ok {
					delete(clients, client)
					close(client.Send)
					if len(clients) == 0 {
						delete(h.boards, client.BoardID)
					}
				}
			}
			h.mu.Unlock()

			count, _ := h.redis.DecrementConnections(client.BoardID)
			h.broadcastConnectionCount(client.BoardID, count)
			log.Printf("Client %s disconnected from board %s", client.ID, client.BoardID)

		case message := <-h.broadcast:
			h.mu.RLock()
			clients := h.boards[message.BoardID]
			h.mu.RUnlock()

			data, err := json.Marshal(message)
			if err != nil {
				log.Printf("Error marshaling message: %v", err)
				continue
			}

			for client := range clients {
				select {
				case client.Send <- data:
				default:
					close(client.Send)
					h.mu.Lock()
					delete(h.boards[message.BoardID], client)
					h.mu.Unlock()
				}
			}
		}
	}
}

func (h *Hub) broadcastConnectionCount(boardID string, count int64) {
	message := Message{
		BoardID: boardID,
		Type:    "connections",
		Data: map[string]interface{}{
			"count": count,
		},
	}
	h.broadcast <- message
}

func (h *Hub) SubscribeToRedis() {
	go func() {
		for boardID := range h.boards {
			go h.subscribeToBoard(boardID)
		}
	}()
}

func (h *Hub) subscribeToBoard(boardID string) {
	pubsub := h.redis.SubscribeToBoard(boardID)
	defer pubsub.Close()

	ch := pubsub.Channel()
	for msg := range ch {
		var update models.BoardUpdate
		if err := json.Unmarshal([]byte(msg.Payload), &update); err != nil {
			log.Printf("Error unmarshaling redis message: %v", err)
			continue
		}

		message := Message{
			BoardID: boardID,
			Type:    "update",
			Data:    update,
		}
		h.broadcast <- message
	}
}