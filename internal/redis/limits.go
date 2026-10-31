package redis

import (
	"context"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// One expiring key per peer and policy, shared by all app instances. Denied
// requests do not extend the window or increment an unbounded counter.
var limitScript = goredis.NewScript(`
local count = tonumber(redis.call('GET', KEYS[1]) or '0')
if count >= tonumber(ARGV[1]) then
  return {0, redis.call('PTTL', KEYS[1])}
end
count = redis.call('INCR', KEYS[1])
if count == 1 then redis.call('PEXPIRE', KEYS[1], ARGV[2]) end
return {1, redis.call('PTTL', KEYS[1])}
`)

func (c *Client) Take(ctx context.Context, key string, limit int, window time.Duration) (bool, time.Duration, error) {
	result, err := limitScript.Run(ctx, c.Client, []string{key}, limit, window.Milliseconds()).Slice()
	if err != nil {
		return false, 0, err
	}
	if len(result) != 2 {
		return false, 0, fmt.Errorf("invalid limiter response")
	}
	allowed, ok := result[0].(int64)
	ttl, ttlOK := result[1].(int64)
	if !ok || !ttlOK || ttl < 0 {
		return false, 0, fmt.Errorf("invalid limiter state")
	}
	return allowed == 1, time.Duration(ttl) * time.Millisecond, nil
}
