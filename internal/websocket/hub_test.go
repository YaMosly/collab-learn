package websocket

import (
	"collab-learn/internal/models"
	appredis "collab-learn/internal/redis"
	"encoding/json"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"
)

func redisClient(t *testing.T, addr string) *appredis.Client {
	t.Helper()
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatal(err)
	}
	c, err := appredis.NewClient(host, port)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}
func runningHub(t *testing.T, c *appredis.Client) *Hub {
	t.Helper()
	h := NewHub(c)
	if err := h.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(h.Close)
	return h
}
func roomClient(t *testing.T, h *Hub, room string) *Client {
	t.Helper()
	c := &Client{ID: uuid.NewString(), BoardID: room, Hub: h, Send: make(chan []byte, 128)}
	if !h.Register(c) {
		t.Fatal("register failed")
	}
	return c
}

type wireMessage struct {
	Type, BoardID string
	Data          json.RawMessage
}

func nextMessage(t *testing.T, c *Client, match func(wireMessage) bool) wireMessage {
	t.Helper()
	timeout := time.NewTimer(5 * time.Second)
	defer timeout.Stop()
	for {
		select {
		case data, ok := <-c.Send:
			if !ok {
				t.Fatal("client disconnected unexpectedly")
			}
			var raw struct {
				Type    string          `json:"type"`
				BoardID string          `json:"board_id"`
				Data    json.RawMessage `json:"data"`
			}
			if err := json.Unmarshal(data, &raw); err != nil {
				t.Fatal(err)
			}
			m := wireMessage{raw.Type, raw.BoardID, raw.Data}
			if match(m) {
				return m
			}
		case <-timeout.C:
			t.Fatal("timed out waiting for event")
		}
	}
}
func updateAt(rev int64) func(wireMessage) bool {
	return func(m wireMessage) bool {
		var b models.Board
		json.Unmarshal(m.Data, &b)
		return m.Type == "update" && b.Revision == rev
	}
}
func countAt(count int) func(wireMessage) bool {
	return func(m wireMessage) bool {
		var data struct {
			Count     int  `json:"count"`
			Available bool `json:"available"`
		}
		json.Unmarshal(m.Data, &data)
		return m.Type == "connections" && data.Available && data.Count == count
	}
}

func TestTwoNodesReceiveNewRoomsAndGlobalPresence(t *testing.T) {
	server := miniredis.RunT(t)
	verifyTwoNodes(t, server.Addr())
}

func TestNativeRedisTwoNodeDelivery(t *testing.T) {
	addr := os.Getenv("COLLABLEARN_TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("set COLLABLEARN_TEST_REDIS_ADDR for an isolated native Redis instance")
	}
	verifyTwoNodes(t, addr)
}

func verifyTwoNodes(t *testing.T, addr string) {
	t.Helper()
	aRedis := redisClient(t, addr)
	bRedis := redisClient(t, addr)
	a, b := runningHub(t, aRedis), runningHub(t, bRedis)
	room := uuid.NewString()
	left, right := roomClient(t, a, room), roomClient(t, b, room)
	nextMessage(t, left, countAt(2))
	nextMessage(t, right, countAt(2))
	board := models.Board{ID: room, Revision: 1, CodeHTML: "from node A", CodeCSS: "CSS"}
	a.BroadcastBoard(&board)
	if err := aRedis.PublishBoardUpdate(room, board); err != nil {
		t.Fatal(err)
	}
	nextMessage(t, left, updateAt(1))
	nextMessage(t, right, updateAt(1))
	board.Revision = 2
	board.CodeHTML = "from node B"
	b.BroadcastBoard(&board)
	if err := bRedis.PublishBoardUpdate(room, board); err != nil {
		t.Fatal(err)
	}
	for _, c := range []*Client{left, right} {
		nextMessage(t, c, func(m wireMessage) bool {
			if m.Type == "update" {
				var got models.Board
				json.Unmarshal(m.Data, &got)
				if got.Revision != 2 {
					t.Fatalf("duplicate/stale update: %+v", got)
				}
				return true
			}
			return false
		})
	}
	a.Unregister(left)
	a.Unregister(left)
	nextMessage(t, right, countAt(1))
}

func TestRevisionDeduplicationAndRoomIsolation(t *testing.T) {
	server := miniredis.RunT(t)
	h := runningHub(t, redisClient(t, server.Addr()))
	room := uuid.NewString()
	c := roomClient(t, h, room)
	for _, rev := range []int64{1, 1, 0, 2} {
		h.BroadcastBoard(&models.Board{ID: room, Revision: rev})
	}
	h.BroadcastBoard(&models.Board{ID: "other-room", Revision: 3})
	h.Send(Message{BoardID: room, Type: "fence"})
	var revisions []int64
	nextMessage(t, c, func(m wireMessage) bool {
		if m.Type == "update" {
			var board models.Board
			json.Unmarshal(m.Data, &board)
			revisions = append(revisions, board.Revision)
		}
		return m.Type == "fence"
	})
	if len(revisions) != 2 || revisions[0] != 1 || revisions[1] != 2 {
		t.Fatal(revisions)
	}
}

func TestMalformedAndMismatchedRedisEventsAreIgnored(t *testing.T) {
	server := miniredis.RunT(t)
	h := runningHub(t, redisClient(t, server.Addr()))
	room := uuid.NewString()
	c := roomClient(t, h, room)
	for _, payload := range []string{`not JSON`, `{"id":"other","revision":3,"code_html":"bad","code_css":""}`, `{"id":"` + room + `","code_html":"no revision","code_css":""}`} {
		h.routeEvent(&goredis.Message{Channel: "board:" + room, Payload: payload})
	}
	h.Send(Message{BoardID: room, Type: "fence"})
	nextMessage(t, c, func(m wireMessage) bool {
		if m.Type == "update" {
			t.Fatal("invalid event accepted")
		}
		return m.Type == "fence"
	})
}

func TestSlowConsumerDoesNotBlockOtherRoomsOrDoubleUnregister(t *testing.T) {
	server := miniredis.RunT(t)
	h := runningHub(t, redisClient(t, server.Addr()))
	slow := &Client{ID: uuid.NewString(), BoardID: uuid.NewString(), Send: make(chan []byte, 1), Hub: h}
	if !h.Register(slow) {
		t.Fatal("register")
	}
	fast := roomClient(t, h, uuid.NewString())
	for i := 0; i < 4; i++ {
		h.Send(Message{BoardID: slow.BoardID, Type: "probe"})
	}
	h.Send(Message{BoardID: fast.BoardID, Type: "fence"})
	nextMessage(t, fast, func(m wireMessage) bool { return m.Type == "fence" })
	if _, ok := h.snapshot()[slow.ID]; ok {
		t.Fatal("slow consumer not evicted")
	}
	h.Unregister(slow)
	h.Unregister(slow)
	h.Send(Message{BoardID: fast.BoardID, Type: "still alive"})
	nextMessage(t, fast, func(m wireMessage) bool { return m.Type == "still alive" })
}

func TestFullQueueAndShutdownNeverBlockProducers(t *testing.T) {
	h := NewHub(nil)
	for i := 0; i < cap(h.broadcast); i++ {
		if !h.Send(Message{}) {
			t.Fatal("early overflow")
		}
	}
	if h.Send(Message{}) {
		t.Fatal("overflow accepted")
	}
	h.Close()
	h.Close()
	if h.Send(Message{}) {
		t.Fatal("send after close accepted")
	}
	if h.Register(&Client{ID: "c", BoardID: "b", Send: make(chan []byte, 1)}) {
		t.Fatal("register after close")
	}
}

func TestRedisResubscriptionClosesClientsToRecoverMissedUpdates(t *testing.T) {
	server := miniredis.RunT(t)
	rdb := redisClient(t, server.Addr())
	h := runningHub(t, rdb)
	c := roomClient(t, h, uuid.NewString())
	nextMessage(t, c, countAt(1))
	server.Close()
	if err := server.Restart(); err != nil {
		t.Fatal(err)
	}
	deadline := time.NewTimer(8 * time.Second)
	defer deadline.Stop()
	closed := false
	for !closed {
		select {
		case _, ok := <-c.Send:
			closed = !ok
		case <-deadline.C:
			t.Fatal("client did not resync after Redis reconnect")
		}
	}
	fresh := roomClient(t, h, c.BoardID)
	if err := rdb.PublishBoardUpdate(c.BoardID, models.Board{ID: c.BoardID, Revision: 7}); err != nil {
		t.Fatal(err)
	}
	nextMessage(t, fresh, updateAt(7))
}

func TestSubscriptionFailurePreventsHubStartup(t *testing.T) {
	server := miniredis.RunT(t)
	c := redisClient(t, server.Addr())
	server.SetError("ERR subscription denied")
	h := NewHub(c)
	if err := h.Start(); err == nil {
		t.Fatal("startup unexpectedly succeeded")
	}
	h.Close()
}

func TestRedisErrorsDoNotBlockLocalRoomDelivery(t *testing.T) {
	server := miniredis.RunT(t)
	h := runningHub(t, redisClient(t, server.Addr()))
	server.SetError("ERR unavailable")
	c := roomClient(t, h, uuid.NewString())
	h.BroadcastBoard(&models.Board{ID: c.BoardID, Revision: 1})
	nextMessage(t, c, updateAt(1))
	server.SetError("")
}

func TestConcurrentCloseAndSend(t *testing.T) {
	server := miniredis.RunT(t)
	h := runningHub(t, redisClient(t, server.Addr()))
	c := roomClient(t, h, uuid.NewString())
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
				h.BroadcastBoard(&models.Board{ID: c.BoardID, Revision: int64(j)})
			}
		}()
	}
	wg.Add(2)
	go func() { defer wg.Done(); h.Close() }()
	go func() { defer wg.Done(); h.Close() }()
	finished := make(chan struct{})
	go func() { wg.Wait(); close(finished) }()
	select {
	case <-finished:
	case <-time.After(8 * time.Second):
		t.Fatal("shutdown blocked")
	}
}

func TestPeriodicResyncIsSentWithoutRedisMessages(t *testing.T) {
	server := miniredis.RunT(t)
	h := NewHub(redisClient(t, server.Addr()))
	h.resyncInterval = 10 * time.Millisecond
	if err := h.Start(); err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	c := roomClient(t, h, uuid.NewString())
	nextMessage(t, c, func(m wireMessage) bool { return m.Type == "resync" })
}
