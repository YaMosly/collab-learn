package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type Client struct {
	*redis.Client
	ctx context.Context
}

func NewClient(host, port string) (*Client, error) {
	ctx := context.Background()
	
	rdb := redis.NewClient(&redis.Options{
		Addr:         fmt.Sprintf("%s:%s", host, port),
		Password:     "",
		DB:           0,
		PoolSize:     10,
		MinIdleConns: 2,
		MaxRetries:   3,
	})

	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, err
	}

	return &Client{
		Client: rdb,
		ctx:    ctx,
	}, nil
}

func (c *Client) PublishBoardUpdate(boardID string, update interface{}) error {
	data, err := json.Marshal(update)
	if err != nil {
		return err
	}

	channel := fmt.Sprintf("board:%s", boardID)
	return c.Publish(c.ctx, channel, data).Err()
}

func (c *Client) SubscribeToBoard(boardID string) *redis.PubSub {
	channel := fmt.Sprintf("board:%s", boardID)
	return c.Subscribe(c.ctx, channel)
}

func (c *Client) IncrementConnections(boardID string) (int64, error) {
	key := fmt.Sprintf("connections:%s", boardID)
	return c.Incr(c.ctx, key).Result()
}

func (c *Client) DecrementConnections(boardID string) (int64, error) {
	key := fmt.Sprintf("connections:%s", boardID)
	return c.Decr(c.ctx, key).Result()
}

func (c *Client) GetConnections(boardID string) (int64, error) {
	key := fmt.Sprintf("connections:%s", boardID)
	val, err := c.Get(c.ctx, key).Result()
	if err == redis.Nil {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	
	var count int64
	fmt.Sscanf(val, "%d", &count)
	return count, nil
}

func (c *Client) CacheBoard(boardID string, data interface{}, expiration time.Duration) error {
	key := fmt.Sprintf("board:cache:%s", boardID)
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return c.Set(c.ctx, key, jsonData, expiration).Err()
}

func (c *Client) GetCachedBoard(boardID string, dest interface{}) error {
	key := fmt.Sprintf("board:cache:%s", boardID)
	val, err := c.Get(c.ctx, key).Result()
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(val), dest)
}