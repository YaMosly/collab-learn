package redis

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
)

func testClient(t *testing.T) (*miniredis.Miniredis, *Client) {
	t.Helper()
	server := miniredis.RunT(t)
	host, port, err := net.SplitHostPort(server.Addr())
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewClient(host, port)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { client.Close() })
	return server, client
}

func TestPresenceLeasesExpireAndRemovalIsIdempotent(t *testing.T) {
	server, c := testClient(t)
	ctx := context.Background()
	now := time.Unix(100000, 0)
	server.SetTime(now)
	for _, id := range []string{"a", "b"} {
		if err := c.UpdatePresence(ctx, "room", id, true, true); err != nil {
			t.Fatal(err)
		}
	}
	if n := c.ZCard(ctx, "connections:v2:room").Val(); n != 2 {
		t.Fatal(n)
	}
	server.SetTime(now.Add(PresenceTTL + time.Second))
	if err := c.UpdatePresence(ctx, "room", "b", true, true); err != nil {
		t.Fatal(err)
	}
	if n := c.ZCard(ctx, "connections:v2:room").Val(); n != 1 {
		t.Fatal(n)
	}
	for i := 0; i < 2; i++ {
		if err := c.UpdatePresence(ctx, "room", "a", false, false); err != nil {
			t.Fatal(err)
		}
	}
	if n := c.ZCard(ctx, "connections:v2:room").Val(); n != 1 {
		t.Fatal(n)
	}
	if err := c.UpdatePresence(ctx, "room", "b", false, false); err != nil {
		t.Fatal(err)
	}
	if server.Exists("connections:v2:room") {
		t.Fatal("empty presence key retained")
	}
}

func TestAbandonedRoomHasBoundedLifetimeAndDoesNotReuseOldCounters(t *testing.T) {
	server, c := testClient(t)
	ctx := context.Background()
	server.Set("connections:room", "99")
	if err := c.UpdatePresence(ctx, "room", "a", true, true); err != nil {
		t.Fatal(err)
	}
	server.FastForward(76 * time.Second)
	if server.Exists("connections:v2:room") {
		t.Fatal("abandoned lease did not expire")
	}
	if value, _ := server.Get("connections:room"); value != "99" {
		t.Fatal("legacy key modified")
	}
}

func TestSubscriptionReadinessCoversBothPatterns(t *testing.T) {
	_, c := testClient(t)
	sub, err := c.SubscribeEvents(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	if n := c.PubSubNumPat(context.Background()).Val(); n != 2 {
		t.Fatalf("ready patterns=%d", n)
	}
}

func TestPresenceBatchRenewsAndRemovesMultipleClients(t *testing.T) {
	_, c := testClient(t)
	ctx := context.Background()
	changes := []PresenceChange{{BoardID: "a", ClientID: "one", Renew: true, Announce: true}, {BoardID: "a", ClientID: "two", Renew: true}, {BoardID: "b", ClientID: "three", Renew: true, Announce: true}}
	if err := c.UpdatePresenceBatch(ctx, changes); err != nil {
		t.Fatal(err)
	}
	if c.ZCard(ctx, "connections:v2:a").Val() != 2 || c.ZCard(ctx, "connections:v2:b").Val() != 1 {
		t.Fatal("wrong batched presence")
	}
	for i := range changes {
		changes[i].Renew = false
	}
	if err := c.UpdatePresenceBatch(ctx, changes); err != nil {
		t.Fatal(err)
	}
	if c.Exists(ctx, "connections:v2:a", "connections:v2:b").Val() != 0 {
		t.Fatal("batch cleanup retained keys")
	}
}
