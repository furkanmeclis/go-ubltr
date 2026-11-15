package sqlstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/furkanmeclis/go-ubltr/provider"
	"github.com/furkanmeclis/go-ubltr/provider/providertest"
	"github.com/furkanmeclis/go-ubltr/provider/watch"
)

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) Now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *clock) Add(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

func setup(t *testing.T) (*Store, *clock, *sql.DB) {
	t.Helper()
	dsn := "file:" + filepath.Join(t.TempDir(), "watch.db") + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	c := &clock{t: time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)}
	s, err := New(db, Options{Dialect: SQLite, Now: c.Now})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
	return s, c, db
}

func TestRebindAndOptions(t *testing.T) {
	t.Parallel()

	if got := Postgres.rebind("a = ? AND b = ?"); got != "a = $1 AND b = $2" {
		t.Fatalf("postgres = %q", got)
	}
	if got := MySQL.rebind("a = ?"); got != "a = ?" {
		t.Fatalf("mysql = %q", got)
	}
	if _, err := New(&sql.DB{}, Options{Prefix: "x; DROP"}); err == nil {
		t.Fatal("bad prefix accepted")
	}
	if _, err := New(nil, Options{}); err == nil {
		t.Fatal("nil db accepted")
	}
}

func TestCursorDedupWatchList(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	s, c, _ := setup(t)
	if v, err := s.Load(ctx, "k"); err != nil || v != "" {
		t.Fatalf("Load = %q, %v", v, err)
	}
	for _, v := range []string{"v1", "v2", "v2"} {
		if err := s.Save(ctx, "k", v); err != nil {
			t.Fatal(err)
		}
	}
	if v, _ := s.Load(ctx, "k"); v != "v2" {
		t.Fatalf("Load = %q", v)
	}

	if seen, err := s.Seen(ctx, "e", time.Minute); err != nil || seen {
		t.Fatalf("Seen = %v, %v", seen, err)
	}
	if seen, err := s.Seen(ctx, "e", time.Minute); err != nil || !seen {
		t.Fatalf("second Seen = %v, %v", seen, err)
	}
	c.Add(2 * time.Minute)
	if seen, _ := s.Seen(ctx, "e", time.Minute); seen {
		t.Fatal("Seen after expiry")
	}
	if err := s.Forget(ctx, "e"); err != nil {
		t.Fatal(err)
	}
	if seen, _ := s.Seen(ctx, "e", time.Minute); seen {
		t.Fatal("Seen after Forget")
	}
	c.Add(time.Hour)
	if err := s.Prune(ctx); err != nil {
		t.Fatal(err)
	}

	t0 := c.Now()
	_ = s.Add(ctx, "w", watch.WatchItem{UUID: "b", AddedAt: t0})
	_ = s.Add(ctx, "w", watch.WatchItem{UUID: "a", AddedAt: t0.Add(time.Second)})
	if err := s.Add(ctx, "w", watch.WatchItem{UUID: "b", AddedAt: t0, Status: provider.StatusSent}); err != nil {
		t.Fatal(err)
	}
	items, err := s.List(ctx, "w")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].UUID != "b" || items[0].Status != provider.StatusSent || items[1].UUID != "a" {
		t.Fatalf("List = %+v", items)
	}
	_ = s.Remove(ctx, "w", "b")
	if items, _ := s.List(ctx, "w"); len(items) != 1 {
		t.Fatalf("after Remove = %+v", items)
	}
	if err := s.Add(ctx, "w", watch.WatchItem{}); err == nil {
		t.Fatal("empty UUID accepted")
	}
}

func TestConcurrentSeen(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	s, _, _ := setup(t)
	var fresh atomic.Int32
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			seen, err := s.Seen(ctx, "same", time.Hour)
			if err != nil {
				t.Error(err)
				return
			}
			if !seen {
				fresh.Add(1)
			}
		})
	}
	wg.Wait()
	if fresh.Load() != 1 {
		t.Fatalf("%d callers saw the ID as new, want 1", fresh.Load())
	}
}

func TestLocker(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	s, c, _ := setup(t)
	unlock, err := s.Lock(ctx, "k", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Lock(ctx, "k", time.Minute); !errors.Is(err, watch.ErrLocked) {
		t.Fatalf("second Lock = %v", err)
	}
	if err := unlock(ctx); err != nil {
		t.Fatal(err)
	}
	stale, err := s.Lock(ctx, "k", time.Minute)
	if err != nil {
		t.Fatalf("Lock after unlock: %v", err)
	}
	c.Add(2 * time.Minute)
	fresh, err := s.Lock(ctx, "k", time.Minute)
	if err != nil {
		t.Fatalf("takeover after expiry: %v", err)
	}
	if err := stale(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Lock(ctx, "k", time.Minute); !errors.Is(err, watch.ErrLocked) {
		t.Fatalf("stale unlock released the lease: %v", err)
	}
	_ = fresh(ctx)
	if _, err := s.Lock(ctx, "k", 0); err == nil {
		t.Fatal("zero ttl accepted")
	}

	var got atomic.Int32
	var wg sync.WaitGroup
	for range 6 {
		wg.Go(func() {
			if _, err := s.Lock(ctx, "race", time.Minute); err == nil {
				got.Add(1)
			} else if !errors.Is(err, watch.ErrLocked) {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if got.Load() != 1 {
		t.Fatalf("%d holders, want 1", got.Load())
	}
}

func event(uuid string) watch.Event {
	return watch.Event{
		ID: watch.EventID("fake", "", provider.Inbound, uuid, provider.StatusReceived), Type: watch.EventInvoiceReceived,
		Provider: "fake", Kind: provider.EInvoice, Direction: provider.Inbound, UUID: uuid, Status: provider.StatusReceived,
	}
}

func TestOutboxSink(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	s, c, db := setup(t)
	o := s.Outbox()
	for i := range 3 {
		c.Add(time.Second)
		if err := o.Publish(ctx, event(fmt.Sprint(i))); err != nil {
			t.Fatal(err)
		}
	}
	if err := o.Publish(ctx, event("0")); err != nil {
		t.Fatalf("duplicate publish: %v", err)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := o.PublishTx(ctx, tx, event("tx")); err != nil {
		t.Fatal(err)
	}
	_ = tx.Rollback()

	pending, err := o.Pending(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 3 || pending[0].Event.UUID != "0" || pending[2].Event.UUID != "2" {
		t.Fatalf("pending = %+v", pending)
	}
	if err := o.MarkPublished(ctx, pending[0].Event.ID, pending[1].Event.ID); err != nil {
		t.Fatal(err)
	}
	pending, _ = o.Pending(ctx, 0)
	if len(pending) != 1 || pending[0].Event.UUID != "2" {
		t.Fatalf("pending after mark = %+v", pending)
	}
	if err := o.Publish(ctx, watch.Event{}); err == nil {
		t.Fatal("event without ID accepted")
	}
}

func TestRunnerOnSQL(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	s, _, _ := setup(t)
	s.now = time.Now
	f := providertest.NewFake()
	for i := range 5 {
		f.Deliver(provider.DocumentInfo{Kind: provider.EInvoice, UUID: fmt.Sprintf("u%d", i), CreatedAt: time.Now().Add(-time.Minute)}, nil)
	}
	run := func() {
		ip, err := watch.NewInboxPoller(f, watch.InboxConfig{Kinds: []provider.DocumentKind{provider.EInvoice}, Cursors: s, Dedup: s, PageSize: 2})
		if err != nil {
			t.Fatal(err)
		}
		r := &watch.Runner{Sink: s.Outbox(), Locker: s}
		_ = r.Add(watch.Job{Name: "inbox", Provider: f, Source: ip})
		if err := r.RunOnce(ctx); err != nil {
			t.Fatal(err)
		}
	}
	run()
	run()
	pending, err := s.Outbox().Pending(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 5 {
		t.Fatalf("outbox rows = %d, want 5", len(pending))
	}
}
