package redisstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/furkanmeclis/go-ubltr/provider"
	"github.com/furkanmeclis/go-ubltr/provider/providertest"
	"github.com/furkanmeclis/go-ubltr/provider/watch"
)

func setup(t *testing.T) (*miniredis.Miniredis, *redis.Client, *Store) {
	t.Helper()
	mr := miniredis.RunT(t)
	c := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = c.Close() })
	return mr, c, New(c, Options{})
}

func TestCursorAndDedup(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	mr, _, s := setup(t)
	if v, err := s.Load(ctx, "k"); err != nil || v != "" {
		t.Fatalf("Load = %q, %v", v, err)
	}
	if err := s.Save(ctx, "k", "v"); err != nil {
		t.Fatal(err)
	}
	if v, _ := s.Load(ctx, "k"); v != "v" {
		t.Fatalf("Load = %q", v)
	}
	if got, _ := mr.Get(DefaultPrefix + "cursor:k"); got != "v" {
		t.Fatalf("raw key = %q", got)
	}

	if seen, err := s.Seen(ctx, "e", time.Minute); err != nil || seen {
		t.Fatalf("Seen = %v, %v", seen, err)
	}
	if seen, _ := s.Seen(ctx, "e", time.Minute); !seen {
		t.Fatal("second Seen = false")
	}
	if ttl := mr.TTL(DefaultPrefix + "seen:e"); ttl != time.Minute {
		t.Fatalf("ttl = %v", ttl)
	}
	mr.FastForward(2 * time.Minute)
	if seen, _ := s.Seen(ctx, "e", time.Minute); seen {
		t.Fatal("Seen after expiry")
	}
	if err := s.Forget(ctx, "e"); err != nil {
		t.Fatal(err)
	}
	if seen, _ := s.Seen(ctx, "e", time.Minute); seen {
		t.Fatal("Seen after Forget")
	}
}

func TestWatchList(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	_, _, s := setup(t)
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	_ = s.Add(ctx, "w", watch.WatchItem{UUID: "b", AddedAt: t0})
	_ = s.Add(ctx, "w", watch.WatchItem{UUID: "a", AddedAt: t0.Add(time.Second)})
	if err := s.Add(ctx, "w", watch.WatchItem{UUID: "b", AddedAt: t0, Status: provider.StatusSent}); err != nil {
		t.Fatal(err)
	}
	items, err := s.List(ctx, "w")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].UUID != "b" || items[0].Status != provider.StatusSent {
		t.Fatalf("List = %+v", items)
	}
	_ = s.Remove(ctx, "w", "b")
	if items, _ := s.List(ctx, "w"); len(items) != 1 || items[0].UUID != "a" {
		t.Fatalf("after Remove = %+v", items)
	}
	if err := s.Add(ctx, "w", watch.WatchItem{}); err == nil {
		t.Fatal("empty UUID accepted")
	}
}

func TestLocker(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	mr, _, s := setup(t)
	unlock, err := s.Lock(ctx, "k", 500*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if ttl := mr.TTL(DefaultPrefix + "lock:k"); ttl != 500*time.Millisecond {
		t.Fatalf("lock ttl = %v (want PX)", ttl)
	}
	if _, err := s.Lock(ctx, "k", time.Second); !errors.Is(err, watch.ErrLocked) {
		t.Fatalf("second Lock = %v", err)
	}
	if err := unlock(ctx); err != nil {
		t.Fatal(err)
	}
	unlock2, err := s.Lock(ctx, "k", time.Second)
	if err != nil {
		t.Fatalf("Lock after unlock: %v", err)
	}
	mr.FastForward(2 * time.Second)
	unlock3, err := s.Lock(ctx, "k", time.Minute)
	if err != nil {
		t.Fatalf("Lock after expiry: %v", err)
	}
	// The expired holder must not delete the new lease.
	if err := unlock2(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Lock(ctx, "k", time.Minute); !errors.Is(err, watch.ErrLocked) {
		t.Fatalf("stale unlock released the lease: %v", err)
	}
	if err := unlock3(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Lock(ctx, "k", 0); err == nil {
		t.Fatal("zero ttl accepted")
	}
}

func sample(uuid string) watch.Event {
	return watch.Event{
		ID: watch.EventID("fake", "", provider.Inbound, uuid, provider.StatusReceived), Type: watch.EventInvoiceReceived,
		Provider: "fake", Kind: provider.EInvoice, Direction: provider.Inbound, UUID: uuid, Status: provider.StatusReceived,
	}
}

func TestStreamSink(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	_, c, _ := setup(t)
	sink := &StreamSink{Client: c, Stream: "events", MaxLen: 1000}
	for i := range 3 {
		if err := sink.Publish(ctx, sample(fmt.Sprint(i))); err != nil {
			t.Fatal(err)
		}
	}
	msgs, err := c.XRange(ctx, "events", "-", "+").Result()
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 3 {
		t.Fatalf("stream = %+v", msgs)
	}
	m := msgs[2].Values
	if m["id"] != sample("2").ID || m["type"] != "invoice.received" {
		t.Fatalf("fields = %v", m)
	}
	var ev watch.Event
	if err := json.Unmarshal([]byte(m["event"].(string)), &ev); err != nil || ev.UUID != "2" {
		t.Fatalf("event = %+v, %v", ev, err)
	}
}

func TestPubSubSink(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	_, c, _ := setup(t)
	sub := c.Subscribe(ctx, "ubltr")
	defer func() { _ = sub.Close() }()
	if _, err := sub.Receive(ctx); err != nil {
		t.Fatal(err)
	}
	if err := (&PubSubSink{Client: c, Channel: "ubltr"}).Publish(ctx, sample("p")); err != nil {
		t.Fatal(err)
	}
	select {
	case msg := <-sub.Channel():
		var ev watch.Event
		if err := json.Unmarshal([]byte(msg.Payload), &ev); err != nil || ev.UUID != "p" {
			t.Fatalf("payload = %q", msg.Payload)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no message")
	}
}

// TestPollerOnRedis runs the inbox poller and runner with Redis state.
func TestPollerOnRedis(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	_, c, s := setup(t)
	f := providertest.NewFake()
	for i := range 3 {
		f.Deliver(provider.DocumentInfo{Kind: provider.EInvoice, UUID: fmt.Sprintf("u%d", i), CreatedAt: time.Now().Add(-time.Minute)}, nil)
	}
	ip, err := watch.NewInboxPoller(f, watch.InboxConfig{Kinds: []provider.DocumentKind{provider.EInvoice}, Cursors: s, Dedup: s, PageSize: 2})
	if err != nil {
		t.Fatal(err)
	}
	sink := &StreamSink{Client: c, Stream: "events"}
	r := &watch.Runner{Sink: sink, Locker: s}
	_ = r.Add(watch.Job{Name: "inbox", Provider: f, Source: ip})
	if err := r.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	// A second instance with the same Redis state emits nothing new.
	ip2, _ := watch.NewInboxPoller(f, watch.InboxConfig{Kinds: []provider.DocumentKind{provider.EInvoice}, Cursors: s, Dedup: s, PageSize: 2})
	r2 := &watch.Runner{Sink: sink, Locker: s}
	_ = r2.Add(watch.Job{Name: "inbox", Provider: f, Source: ip2})
	if err := r2.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if n, _ := c.XLen(ctx, "events").Result(); n != 3 {
		t.Fatalf("stream length = %d, want 3", n)
	}
}
