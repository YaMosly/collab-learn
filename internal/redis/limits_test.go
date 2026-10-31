package redis

import (
	"context"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestRequestLimitsAreAtomicAndShared(t *testing.T) {
	server, a := testClient(t)
	host, port, _ := net.SplitHostPort(server.Addr())
	b, err := NewClient(host, port)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	var allowed atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			client := a
			if i%2 == 0 {
				client = b
			}
			ok, _, err := client.Take(context.Background(), "limits:test", 10, time.Minute)
			if err != nil {
				t.Error(err)
			}
			if ok {
				allowed.Add(1)
			}
		}(i)
	}
	wg.Wait()
	if allowed.Load() != 10 {
		t.Fatalf("allowed=%d", allowed.Load())
	}
	server.FastForward(30 * time.Second)
	if ok, retry, err := a.Take(context.Background(), "limits:test", 10, time.Minute); ok || err != nil || retry != 30*time.Second {
		t.Fatalf("denied request extended window: %v %v %v", ok, retry, err)
	}
	server.FastForward(31 * time.Second)
	if ok, _, err := b.Take(context.Background(), "limits:test", 10, time.Minute); !ok || err != nil {
		t.Fatal("window did not expire", err)
	}
}
