package watch

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/furkanmeclis/go-ubltr/provider"
)

func listUUIDs(t *testing.T, tr *OutboxTracker, wl WatchList) map[string]WatchItem {
	t.Helper()
	items, err := wl.List(context.Background(), tr.Key())
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]WatchItem{}
	for _, it := range items {
		out[it.UUID] = it
	}
	return out
}

func TestOutboxTrackerBasic(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	c := newClock()
	f := newFake(c)
	wl := NewMemoryStore()
	tr, err := NewOutboxTracker(f, OutboxConfig{Tenant: "t1", WatchList: wl, Now: c.Now})
	if err != nil {
		t.Fatal(err)
	}
	const (
		basic = "11111111-1111-1111-1111-111111111111"
		comm  = "22222222-2222-2222-2222-222222222222"
	)
	for uuid, profile := range map[string]string{basic: "TEMELFATURA", comm: ProfileCommercial} {
		doc, res := send(t, f, uuid, profile)
		if err := tr.TrackSent(ctx, doc, res); err != nil {
			t.Fatal(err)
		}
	}
	sink := &collector{}

	// No change yet.
	if err := Pipe(ctx, tr, sink); err != nil {
		t.Fatal(err)
	}
	if len(sink.Events()) != 0 {
		t.Fatalf("events without change: %+v", sink.Events())
	}

	_ = f.Advance(basic, provider.StatusSent, 1200)
	_ = f.Advance(comm, provider.StatusDelivered, 1300)
	if err := Pipe(ctx, tr, sink); err != nil {
		t.Fatal(err)
	}
	evs := sink.Events()
	if len(evs) != 2 {
		t.Fatalf("events = %+v", evs)
	}
	for _, ev := range evs {
		if ev.Type != EventStatusChanged || ev.Direction != provider.Outbound || ev.PreviousStatus != provider.StatusQueued {
			t.Fatalf("event = %+v", ev)
		}
	}
	items := listUUIDs(t, tr, wl)
	if items[comm].DeliveredAt.IsZero() || items[basic].Status != provider.StatusSent {
		t.Fatalf("items = %+v", items)
	}

	// Basic invoice settles on delivery; the commercial one waits.
	_ = f.Advance(basic, provider.StatusDelivered, 1300)
	if err := Pipe(ctx, tr, sink); err != nil {
		t.Fatal(err)
	}
	items = listUUIDs(t, tr, wl)
	if _, ok := items[basic]; ok {
		t.Fatal("delivered TEMELFATURA still tracked")
	}
	if _, ok := items[comm]; !ok {
		t.Fatal("delivered TICARIFATURA dropped")
	}

	_ = f.Advance(comm, provider.StatusAccepted, 1300)
	if err := Pipe(ctx, tr, sink); err != nil {
		t.Fatal(err)
	}
	last := sink.Events()[len(sink.Events())-1]
	if last.Type != EventResponseReceived || last.Status != provider.StatusAccepted || last.ID != EventID("fake", "t1", provider.Outbound, comm, provider.StatusAccepted) {
		t.Fatalf("response event = %+v", last)
	}
	if len(listUUIDs(t, tr, wl)) != 0 {
		t.Fatal("watch list not empty")
	}
}

func TestOutboxTrackerResponseWindow(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	c := newClock()
	f := newFake(c)
	wl := NewMemoryStore()
	tr, _ := NewOutboxTracker(f, OutboxConfig{WatchList: wl, Now: c.Now})
	const id = "33333333-3333-3333-3333-333333333333"
	doc, res := send(t, f, id, ProfileCommercial)
	if err := tr.TrackSent(ctx, doc, res); err != nil {
		t.Fatal(err)
	}
	_ = f.Advance(id, provider.StatusDelivered, 1300)
	sink := &collector{}
	if err := Pipe(ctx, tr, sink); err != nil {
		t.Fatal(err)
	}
	c.Add(7 * 24 * time.Hour)
	if err := Pipe(ctx, tr, sink); err != nil {
		t.Fatal(err)
	}
	if len(listUUIDs(t, tr, wl)) != 1 {
		t.Fatal("dropped inside the response window")
	}
	c.Add(25 * time.Hour)
	if err := Pipe(ctx, tr, sink); err != nil {
		t.Fatal(err)
	}
	if len(listUUIDs(t, tr, wl)) != 0 {
		t.Fatal("still tracked after the response window")
	}
	if len(sink.Events()) != 1 {
		t.Fatalf("events = %+v", sink.Events())
	}
}

func TestOutboxTrackerEArchiveCancel(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	c := newClock()
	f := newFake(c)
	wl := NewMemoryStore()
	tr, _ := NewOutboxTracker(f, OutboxConfig{WatchList: wl, WaitTerminal: true, Now: c.Now})
	const id = "44444444-4444-4444-4444-444444444444"
	doc, res := send(t, f, id, "EARSIVFATURA")
	if doc.Kind != provider.EArchive {
		t.Fatalf("kind = %s", doc.Kind)
	}
	if err := tr.TrackSent(ctx, doc, res); err != nil {
		t.Fatal(err)
	}
	_ = f.Advance(id, provider.StatusDelivered, 1300)
	sink := &collector{}
	if err := Pipe(ctx, tr, sink); err != nil {
		t.Fatal(err)
	}
	if len(listUUIDs(t, tr, wl)) != 1 {
		t.Fatal("WaitTerminal dropped a delivered e-Arşiv invoice")
	}
	if err := f.CancelEArchive(ctx, id, "test"); err != nil {
		t.Fatal(err)
	}
	if err := Pipe(ctx, tr, sink); err != nil {
		t.Fatal(err)
	}
	evs := sink.Events()
	if len(evs) != 2 || evs[1].Type != EventEArchiveCancelled || evs[1].Kind != provider.EArchive {
		t.Fatalf("events = %+v", evs)
	}
	if len(listUUIDs(t, tr, wl)) != 0 {
		t.Fatal("cancelled invoice still tracked")
	}
}

func TestOutboxTrackerErrors(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	c := newClock()
	f := newFake(c)
	wl := NewMemoryStore()
	tr, _ := NewOutboxTracker(f, OutboxConfig{WatchList: wl, Now: c.Now})

	// Unknown to the integrator: kept until MaxAge.
	if err := tr.Track(ctx, WatchItem{UUID: "missing"}); err != nil {
		t.Fatal(err)
	}
	// Sink failure: the status is not recorded, so it is emitted again.
	const id = "55555555-5555-5555-5555-555555555555"
	doc, res := send(t, f, id, "TEMELFATURA")
	_ = tr.TrackSent(ctx, doc, res)
	_ = f.Advance(id, provider.StatusSent, 1200)
	if err := Pipe(ctx, tr, &collector{failAt: 1}); err == nil {
		t.Fatal("expected sink error")
	}
	sink := &collector{}
	if err := Pipe(ctx, tr, sink); err != nil {
		t.Fatal(err)
	}
	if len(sink.Events()) != 1 || sink.Events()[0].UUID != id {
		t.Fatalf("retry events = %+v", sink.Events())
	}
	if _, ok := listUUIDs(t, tr, wl)["missing"]; !ok {
		t.Fatal("not-found document dropped early")
	}
	c.Add(DefaultMaxAge)
	if err := Pipe(ctx, tr, sink); err != nil {
		t.Fatal(err)
	}
	if len(listUUIDs(t, tr, wl)) != 0 {
		t.Fatal("MaxAge did not drop documents")
	}

	// Retryable integrator errors stop the pass.
	rl := rateLimited{capsProvider{Fake: f, caps: f.Capabilities()}}
	tr2, _ := NewOutboxTracker(rl, OutboxConfig{WatchList: wl, Now: c.Now})
	_ = tr2.Track(ctx, WatchItem{UUID: "x"})
	err := Pipe(ctx, tr2, sink)
	if !errors.Is(err, provider.ErrRateLimit) {
		t.Fatalf("err = %v", err)
	}
	if err := tr2.Untrack(ctx, "x"); err != nil {
		t.Fatal(err)
	}
	if err := tr.Track(ctx, WatchItem{}); err == nil {
		t.Fatal("Track without UUID")
	}
	if _, err := NewOutboxTracker(nil, OutboxConfig{}); err == nil {
		t.Fatal("nil provider accepted")
	}
}

type rateLimited struct{ capsProvider }

func (rateLimited) Status(context.Context, provider.StatusQuery) (*provider.StatusResult, error) {
	return nil, &provider.Error{Kind: provider.KindRateLimit, Provider: "fake"}
}
