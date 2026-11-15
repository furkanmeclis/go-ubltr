package watch

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/furkanmeclis/go-ubltr/provider"
)

func TestEventJSONFieldNames(t *testing.T) {
	t.Parallel()

	ev := Event{
		ID: EventID("fake", "t1", provider.Inbound, "u1", provider.StatusReceived), Type: EventInvoiceReceived,
		Provider: "fake", Tenant: "t1", Kind: provider.EInvoice, Direction: provider.Inbound, UUID: "u1",
		DocumentID: "ABC1", Status: provider.StatusReceived, GIBCode: 1300, ProviderCode: "1300",
		Info: provider.DocumentInfo{UUID: "u1"}, At: time.Unix(0, 0).UTC(),
	}
	if ev.ID != "fake|t1|inbound|u1|received" {
		t.Fatalf("ID = %q", ev.ID)
	}
	b, err := json.Marshal(ev)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"id", "type", "provider", "tenant", "kind", "direction", "uuid", "document_id", "status", "gib_code", "provider_code", "info", "at"} {
		if _, ok := m[k]; !ok {
			t.Errorf("missing JSON field %q in %s", k, b)
		}
	}
	if info, _ := m["info"].(map[string]any); info["uuid"] != "u1" || len(info) != 1 {
		t.Errorf("info = %v, want snake_case with empty fields omitted", m["info"])
	}
	var back Event
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(back, ev) {
		t.Fatalf("round trip:\n got %+v\nwant %+v", back, ev)
	}
	b, _ = json.Marshal(Event{ID: "x"})
	var zero map[string]any
	if err := json.Unmarshal(b, &zero); err != nil {
		t.Fatal(err)
	}
	if _, ok := zero["info"]; ok {
		t.Errorf("zero Info should be omitted: %s", b)
	}
}

func testStore(t *testing.T, s *Store, c *clock) {
	t.Helper()
	ctx := context.Background()
	s.SetClock(c.Now)

	if v, err := s.Load(ctx, "k"); err != nil || v != "" {
		t.Fatalf("Load empty = %q, %v", v, err)
	}
	if err := s.Save(ctx, "k", "v1"); err != nil {
		t.Fatal(err)
	}
	if v, _ := s.Load(ctx, "k"); v != "v1" {
		t.Fatalf("Load = %q", v)
	}

	seen, err := s.Seen(ctx, "e1", time.Hour)
	if err != nil || seen {
		t.Fatalf("first Seen = %v, %v", seen, err)
	}
	if seen, _ := s.Seen(ctx, "e1", time.Hour); !seen {
		t.Fatal("second Seen = false")
	}
	if err := s.Forget(ctx, "e1"); err != nil {
		t.Fatal(err)
	}
	if seen, _ := s.Seen(ctx, "e1", time.Hour); seen {
		t.Fatal("Seen after Forget = true")
	}
	c.Add(2 * time.Hour)
	if seen, _ := s.Seen(ctx, "e1", time.Hour); seen {
		t.Fatal("Seen after expiry = true")
	}

	t0 := c.Now()
	if err := s.Add(ctx, "w", WatchItem{UUID: "b", AddedAt: t0}); err != nil {
		t.Fatal(err)
	}
	if err := s.Add(ctx, "w", WatchItem{UUID: "a", AddedAt: t0.Add(time.Second)}); err != nil {
		t.Fatal(err)
	}
	if err := s.Add(ctx, "w", WatchItem{UUID: "b", AddedAt: t0, Status: provider.StatusSent}); err != nil {
		t.Fatal(err)
	}
	items, _ := s.List(ctx, "w")
	if len(items) != 2 || items[0].UUID != "b" || items[0].Status != provider.StatusSent || items[1].UUID != "a" {
		t.Fatalf("List = %+v", items)
	}
	if err := s.Remove(ctx, "w", "b"); err != nil {
		t.Fatal(err)
	}
	if err := s.Remove(ctx, "w", "missing"); err != nil {
		t.Fatal(err)
	}
	if items, _ := s.List(ctx, "w"); len(items) != 1 || items[0].UUID != "a" {
		t.Fatalf("List after Remove = %+v", items)
	}
	if err := s.Add(ctx, "w", WatchItem{}); err == nil {
		t.Fatal("Add without UUID succeeded")
	}
}

func TestMemoryStore(t *testing.T) {
	t.Parallel()
	testStore(t, NewMemoryStore(), newClock())
}

func TestFileStore(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	s, err := OpenFileStore(path)
	if err != nil {
		t.Fatal(err)
	}
	c := newClock()
	testStore(t, s, c)

	re, err := OpenFileStore(path)
	if err != nil {
		t.Fatal(err)
	}
	re.SetClock(c.Now)
	ctx := context.Background()
	if v, _ := re.Load(ctx, "k"); v != "v1" {
		t.Fatalf("reopened Load = %q", v)
	}
	if seen, _ := re.Seen(ctx, "e1", time.Hour); !seen {
		t.Fatal("reopened Seen = false")
	}
	if items, _ := re.List(ctx, "w"); len(items) != 1 {
		t.Fatalf("reopened List = %+v", items)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("leftover files: %v", entries)
	}
}

func TestFileStoreErrors(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(bad, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenFileStore(bad); err == nil {
		t.Fatal("corrupt state opened")
	}

	// A path in a missing directory fails on write and keeps memory state
	// unchanged.
	s, err := OpenFileStore(filepath.Join(dir, "missing", "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := s.Save(ctx, "k", "v"); err == nil {
		t.Fatal("Save into missing dir succeeded")
	}
	if v, _ := s.Load(ctx, "k"); v != "" {
		t.Fatalf("failed Save left %q", v)
	}
	if _, err := s.Seen(ctx, "e", time.Hour); err == nil {
		t.Fatal("Seen into missing dir succeeded")
	}
	if seen, _ := NewMemoryStore().Seen(ctx, "e", time.Hour); seen {
		t.Fatal("unexpected seen")
	}
}

func TestMemoryLocker(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	l := NewMemoryLocker()
	c := newClock()
	l.clock = c.Now
	unlock, err := l.Lock(ctx, "k", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := l.Lock(ctx, "k", time.Minute); !errors.Is(err, ErrLocked) {
		t.Fatalf("second Lock err = %v", err)
	}
	if _, err := l.Lock(ctx, "other", time.Minute); err != nil {
		t.Fatalf("other key: %v", err)
	}
	if err := unlock(ctx); err != nil {
		t.Fatal(err)
	}
	unlock2, err := l.Lock(ctx, "k", time.Minute)
	if err != nil {
		t.Fatalf("Lock after unlock: %v", err)
	}
	c.Add(2 * time.Minute)
	unlock3, err := l.Lock(ctx, "k", time.Minute)
	if err != nil {
		t.Fatalf("Lock after expiry: %v", err)
	}
	// The stale holder's unlock must not release the new lease.
	_ = unlock2(ctx)
	if _, err := l.Lock(ctx, "k", time.Minute); !errors.Is(err, ErrLocked) {
		t.Fatalf("stale unlock released lease: %v", err)
	}
	_ = unlock3(ctx)
}
