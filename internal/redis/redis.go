package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

const BoardPattern = "board:*"
const PresencePattern = "presence:*"
const PresenceTTL = 60 * time.Second

type Client struct{ *goredis.Client }

func NewClient(host, port string) (*Client, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return Connect(ctx, net.JoinHostPort(host, port))
}

func Connect(ctx context.Context, address string) (*Client, error) {
	rdb := goredis.NewClient(&goredis.Options{Addr: address,
		PoolSize: 10, MinIdleConns: 2, MaxRetries: 1, ContextTimeoutEnabled: true,
		DialTimeout: 2 * time.Second, ReadTimeout: 2 * time.Second, WriteTimeout: 2 * time.Second, PoolTimeout: 2 * time.Second})
	if err := rdb.Ping(ctx).Err(); err != nil {
		rdb.Close()
		return nil, err
	}
	return &Client{rdb}, nil
}

func (c *Client) PublishBoardUpdate(boardID string, update interface{}) error {
	data, err := json.Marshal(update)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return c.Publish(ctx, "board:"+boardID, data).Err()
}

// One acknowledged pattern subscription covers rooms created after startup.
// Receive must not be used after the caller starts ChannelWithSubscriptions.
func (c *Client) SubscribeEvents(ctx context.Context) (*goredis.PubSub, error) {
	pubsub := c.PSubscribe(ctx, BoardPattern, PresencePattern)
	pending := map[string]bool{BoardPattern: true, PresencePattern: true}
	deadline := time.Now().Add(3 * time.Second)
	if parent, ok := ctx.Deadline(); ok && parent.Before(deadline) {
		deadline = parent
	}
	for len(pending) > 0 {
		if err := ctx.Err(); err != nil {
			pubsub.Close()
			return nil, err
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			pubsub.Close()
			return nil, fmt.Errorf("Redis subscription readiness timed out")
		}
		message, err := pubsub.ReceiveTimeout(ctx, remaining)
		if err != nil {
			pubsub.Close()
			return nil, err
		}
		if sub, ok := message.(*goredis.Subscription); ok && sub.Kind == "psubscribe" {
			delete(pending, sub.Channel)
		}
	}
	return pubsub, nil
}

// Redis server time is shared by all nodes. Each WebSocket owns an expiring
// sorted-set member; a lost remove can only leave a temporary lease.
const presenceLua = `
local clock = redis.call('TIME')
local now = tonumber(clock[1]) * 1000 + math.floor(tonumber(clock[2]) / 1000)
local before = redis.call('ZCARD', KEYS[1])
local exists = redis.call('ZSCORE', KEYS[1], ARGV[1])
redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', now)
if ARGV[2] == 'renew' then
  redis.call('ZADD', KEYS[1], now + tonumber(ARGV[4]), ARGV[1])
else
  redis.call('ZREM', KEYS[1], ARGV[1])
end
local count = redis.call('ZCARD', KEYS[1])
if count == 0 then redis.call('DEL', KEYS[1])
else redis.call('PEXPIRE', KEYS[1], tonumber(ARGV[4]) + 15000) end
if before ~= count or (ARGV[2] == 'renew' and not exists) or ARGV[5] == 'announce' then
  redis.call('PUBLISH', ARGV[3], tostring(count))
end
return count
`

var presenceScript = goredis.NewScript(presenceLua)

type PresenceChange struct {
	BoardID, ClientID string
	Renew, Announce   bool
}

func (p PresenceChange) args() []interface{} {
	op, event := "remove", "quiet"
	if p.Renew {
		op = "renew"
	}
	if p.Announce {
		event = "announce"
	}
	return []interface{}{p.ClientID, op, "presence:" + p.BoardID, PresenceTTL.Milliseconds(), event}
}

// Pipeline leases so one slow network round-trip per user cannot starve the
// remaining users' heartbeats. Each individual Lua operation stays atomic.
func (c *Client) UpdatePresenceBatch(ctx context.Context, changes []PresenceChange) error {
	if len(changes) == 0 {
		return nil
	}
	pipe := c.Pipeline()
	for _, change := range changes {
		pipe.Eval(ctx, presenceLua, []string{"connections:v2:" + change.BoardID}, change.args()...)
	}
	_, err := pipe.Exec(ctx)
	return err
}

func (c *Client) UpdatePresence(ctx context.Context, boardID, clientID string, renew, announce bool) error {
	change := PresenceChange{boardID, clientID, renew, announce}
	return presenceScript.Run(ctx, c.Client, []string{"connections:v2:" + boardID}, change.args()...).Err()
}
