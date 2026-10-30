package websocket

import (
	"collab-learn/internal/models"
	"collab-learn/internal/redis"
	"context"
	"encoding/json"
	"errors"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	goredis "github.com/redis/go-redis/v9"
)

type Message struct {
	BoardID   string      `json:"board_id"`
	Type      string      `json:"type"`
	Data      interface{} `json:"data"`
	Recipient *Client     `json:"-"`
}
type Client struct {
	ID, BoardID string
	Send        chan []byte
	Hub         *Hub
	conn        *websocket.Conn
}
type registration struct {
	client   *Client
	accepted chan bool
}

type Hub struct {
	// Room maps are owned exclusively by run; Redis workers request snapshots.
	boards                           map[string]map[*Client]bool
	seen                             map[string]int64
	broadcast                        chan Message
	register                         chan registration
	unregister                       chan *Client
	snapshots                        chan chan map[string]string
	reset                            chan struct{}
	presenceChanged                  chan struct{}
	redis                            *redis.Client
	ctx                              context.Context
	cancel                           context.CancelFunc
	done                             chan struct{}
	lifecycle                        sync.Mutex
	started, closed                  bool
	workers                          sync.WaitGroup
	sub                              *goredis.PubSub
	presenceInterval, resyncInterval time.Duration
}

func NewHub(client *redis.Client) *Hub {
	ctx, cancel := context.WithCancel(context.Background())
	return &Hub{boards: map[string]map[*Client]bool{}, seen: map[string]int64{},
		broadcast: make(chan Message, 256), register: make(chan registration), unregister: make(chan *Client, 256),
		snapshots: make(chan chan map[string]string), reset: make(chan struct{}, 1), presenceChanged: make(chan struct{}, 1),
		redis: client, ctx: ctx, cancel: cancel, done: make(chan struct{}),
		presenceInterval: 15 * time.Second, resyncInterval: 30 * time.Second}
}

func (h *Hub) Start() error {
	return h.StartContext(h.ctx)
}

// The startup deadline only bounds the initial subscription acknowledgement.
// Long-lived workers use h.ctx, independent of the caller's startup deadline.
func (h *Hub) StartContext(ctx context.Context) error {
	h.lifecycle.Lock()
	defer h.lifecycle.Unlock()
	if h.started || h.closed {
		return errors.New("hub already started or closed")
	}
	if h.redis == nil {
		return errors.New("Redis is required")
	}
	sub, err := h.redis.SubscribeEvents(ctx)
	if err != nil {
		return err
	}
	h.sub = sub
	h.started = true
	h.workers.Add(3)
	go func() { defer h.workers.Done(); h.run() }()
	go func() { defer h.workers.Done(); h.readEvents() }()
	go func() { defer h.workers.Done(); h.presenceWorker() }()
	return nil
}

func (h *Hub) Active() bool {
	h.lifecycle.Lock()
	defer h.lifecycle.Unlock()
	return h.started && !h.closed
}

func (h *Hub) Close() {
	h.lifecycle.Lock()
	if !h.closed {
		h.closed = true
		h.cancel()
		if h.sub != nil {
			h.sub.Close()
		}
		if !h.started {
			close(h.done)
		}
	}
	h.lifecycle.Unlock()
	h.workers.Wait()
}

func (h *Hub) Register(c *Client) bool {
	h.lifecycle.Lock()
	active := h.started && !h.closed
	h.lifecycle.Unlock()
	if !active {
		return false
	}
	if c == nil || c.ID == "" || c.BoardID == "" || c.Send == nil {
		return false
	}
	request := registration{c, make(chan bool, 1)}
	select {
	case <-h.ctx.Done():
		return false
	case h.register <- request:
	}
	select {
	case <-h.ctx.Done():
		return false
	case ok := <-request.accepted:
		return ok
	}
}
func (h *Hub) Unregister(c *Client) {
	if c == nil {
		return
	}
	select {
	case <-h.ctx.Done():
	case h.unregister <- c:
	}
}
func (h *Hub) Send(message Message) bool {
	select {
	case <-h.ctx.Done():
		return false
	default:
	}
	select {
	case h.broadcast <- message:
		return true
	default:
		// A missing notification must trigger an authoritative HTTP refresh.
		h.requestReset()
		return false
	}
}
func (h *Hub) BroadcastBoard(board *models.Board) {
	if board == nil {
		return
	}
	h.Send(Message{BoardID: board.ID, Type: "update", Data: *board})
}
func (h *Hub) requestReset() {
	select {
	case h.reset <- struct{}{}:
	default:
	}
}
func (h *Hub) notifyPresence() {
	select {
	case h.presenceChanged <- struct{}{}:
	default:
	}
}

func (h *Hub) snapshot() map[string]string {
	reply := make(chan map[string]string, 1)
	select {
	case <-h.ctx.Done():
		return nil
	case h.snapshots <- reply:
	}
	select {
	case <-h.ctx.Done():
		return nil
	case result := <-reply:
		return result
	}
}

func (h *Hub) remove(c *Client) {
	clients := h.boards[c.BoardID]
	if !clients[c] {
		return
	}
	delete(clients, c)
	close(c.Send)
	if c.conn != nil {
		c.conn.Close()
	}
	if len(clients) == 0 {
		delete(h.boards, c.BoardID)
		delete(h.seen, c.BoardID)
	}
	h.notifyPresence()
}
func (h *Hub) dropAll() {
	for _, clients := range h.boards {
		for c := range clients {
			h.remove(c)
		}
	}
}

func (h *Hub) deliver(message Message) {
	clients := h.boards[message.BoardID]
	if len(clients) == 0 {
		return
	}
	if message.Type == "update" {
		board, ok := message.Data.(models.Board)
		if !ok || board.ID != message.BoardID || board.Revision < 0 {
			return
		}
		if last, ok := h.seen[message.BoardID]; ok && board.Revision <= last {
			return
		}
		h.seen[message.BoardID] = board.Revision
	}
	data, err := json.Marshal(message)
	if err != nil {
		return
	}
	for c := range clients {
		if message.Recipient != nil && c != message.Recipient {
			continue
		}
		select {
		case c.Send <- data:
		default:
			h.remove(c)
		}
	}
}

func (h *Hub) run() {
	defer close(h.done)
	defer h.dropAll()
	ticker := time.NewTicker(h.resyncInterval)
	defer ticker.Stop()
	for {
		select {
		case <-h.ctx.Done():
			return
		case <-h.reset:
			h.dropAll()
		case command := <-h.register:
			c := command.client
			if h.boards[c.BoardID] == nil {
				h.boards[c.BoardID] = map[*Client]bool{}
			}
			h.boards[c.BoardID][c] = true
			command.accepted <- true
			h.notifyPresence()
		case c := <-h.unregister:
			h.remove(c)
		case reply := <-h.snapshots:
			current := map[string]string{}
			for board, clients := range h.boards {
				for c := range clients {
					current[c.ID] = board
				}
			}
			reply <- current
		case message := <-h.broadcast:
			h.deliver(message)
		case <-ticker.C:
			for board := range h.boards {
				h.deliver(Message{BoardID: board, Type: "resync"})
			}
		}
	}
}

func (h *Hub) readEvents() {
	events := h.sub.ChannelWithSubscriptions(goredis.WithChannelSize(256))
	for {
		select {
		case <-h.ctx.Done():
			return
		case value, ok := <-events:
			if !ok {
				h.requestReset()
				return
			}
			switch event := value.(type) {
			case *goredis.Subscription:
				// Initial acknowledgements were consumed by Start. This is a
				// reconnect: Pub/Sub cannot replay writes made during the gap.
				if event.Kind == "psubscribe" && event.Channel == redis.BoardPattern {
					h.requestReset()
				}
			case *goredis.Message:
				h.routeEvent(event)
			}
		}
	}
}

func (h *Hub) routeEvent(event *goredis.Message) {
	if len(event.Payload) > 1024*1024 {
		return
	}
	if strings.HasPrefix(event.Channel, "presence:") {
		board := strings.TrimPrefix(event.Channel, "presence:")
		count, err := strconv.ParseInt(event.Payload, 10, 64)
		if err == nil && count >= 0 && board != "" {
			h.Send(Message{BoardID: board, Type: "connections", Data: map[string]interface{}{"count": count, "available": true}})
		}
		return
	}
	var board models.Board
	var required struct {
		Revision *int64  `json:"revision"`
		HTML     *string `json:"code_html"`
		CSS      *string `json:"code_css"`
	}
	if json.Unmarshal([]byte(event.Payload), &board) != nil || json.Unmarshal([]byte(event.Payload), &required) != nil {
		return
	}
	if board.ID == "" || event.Channel != "board:"+board.ID || required.Revision == nil || required.HTML == nil || required.CSS == nil || board.Revision < 0 {
		return
	}
	h.BroadcastBoard(&board)
}

func (h *Hub) presenceWorker() {
	ticker := time.NewTicker(h.presenceInterval)
	defer ticker.Stop()
	known := map[string]string{}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		changes := make([]redis.PresenceChange, 0, len(known))
		for id, board := range known {
			changes = append(changes, redis.PresenceChange{BoardID: board, ClientID: id})
		}
		// If cleanup cannot reach Redis, the leases still expire.
		h.redis.UpdatePresenceBatch(ctx, changes)
	}()
	for {
		select {
		case <-h.ctx.Done():
			return
		case <-h.presenceChanged:
		case <-ticker.C:
		}
		current := h.snapshot()
		if h.ctx.Err() != nil {
			return
		}
		ctx, cancel := context.WithTimeout(h.ctx, 3*time.Second)
		rooms := map[string]bool{}
		for _, board := range current {
			rooms[board] = true
		}
		changes := make([]redis.PresenceChange, 0, len(known)+len(current))
		for id, board := range known {
			if _, ok := current[id]; ok {
				continue
			}
			changes = append(changes, redis.PresenceChange{BoardID: board, ClientID: id})
		}
		announced := map[string]bool{}
		for id, board := range current {
			known[id] = board // also clean up an uncertain write after a lost reply
			changes = append(changes, redis.PresenceChange{BoardID: board, ClientID: id, Renew: true, Announce: !announced[board]})
			announced[board] = true
		}
		failure := h.redis.UpdatePresenceBatch(ctx, changes)
		if failure == nil {
			for _, change := range changes {
				if !change.Renew {
					delete(known, change.ClientID)
				}
			}
		}
		cancel()
		if failure != nil && h.ctx.Err() == nil {
			log.Printf("Presence refresh unavailable: %v", failure)
			for board := range rooms {
				h.Send(Message{BoardID: board, Type: "connections", Data: map[string]interface{}{"count": 0, "available": false}})
			}
		}
	}
}
