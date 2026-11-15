package watch

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/furkanmeclis/go-ubltr/provider"
)

func einvoiceOnly(cfg InboxConfig) InboxConfig {
	cfg.Kinds = []provider.DocumentKind{provider.EInvoice}
	return cfg
}

func TestInboxPollerBasic(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	c := newClock()
	f := newFake(c)
	for i := range 3 {
		deliver(f, provider.EInvoice, fmt.Sprintf("u%d", i), c.Now().Add(-time.Duration(3-i)*time.Minute))
	}
	store := NewMemoryStore()
	ip, err := NewInboxPoller(f, einvoiceOnly(InboxConfig{Tenant: "t1", Cursors: store, Dedup: store, Now: c.Now}))
	if err != nil {
		t.Fatal(err)
	}
	sink := &collector{}
	if err := Pipe(ctx, ip, sink); err != nil {
		t.Fatal(err)
	}
	evs := sink.Events()
	if got := sink.UUIDs(); !slices.Equal(got, []string{"u0", "u1", "u2"}) {
		t.Fatalf("uuids = %v", got)
	}
	ev := evs[0]
	if ev.Type != EventInvoiceReceived || ev.ID != "fake|t1|inbound|u0|received" || ev.Provider != "fake" ||
		ev.Tenant != "t1" || ev.Direction != provider.Inbound || ev.DocumentID != "ABCu0" || ev.Info.UUID != "u0" || ev.Kind != provider.EInvoice {
		t.Fatalf("event = %+v", ev)
	}

	// Nothing new: no events, watermark moved to the pass end.
	c.Add(time.Minute)
	if err := Pipe(ctx, ip, sink); err != nil {
		t.Fatal(err)
	}
	if n := len(sink.Events()); n != 3 {
		t.Fatalf("second pass emitted %d new events", n-3)
	}

	// A new document and a late one stamped inside the overlap window.
	deliver(f, provider.EInvoice, "new", c.Now())
	deliver(f, provider.EInvoice, "late", c.Now().Add(-5*time.Minute))
	c.Add(time.Minute)
	if err := Pipe(ctx, ip, sink); err != nil {
		t.Fatal(err)
	}
	if got := sink.UUIDs()[3:]; !slices.Equal(got, []string{"late", "new"}) {
		t.Fatalf("third pass = %v", got)
	}
	raw, _ := store.Load(ctx, ip.CursorKey(provider.EInvoice))
	if raw == "" {
		t.Fatal("cursor not saved")
	}
}

func TestInboxPollerPagination(t *testing.T) {
	t.Parallel()

	c := newClock()
	f := newFake(c)
	for i := range 7 {
		deliver(f, provider.EInvoice, fmt.Sprintf("u%d", i), c.Now().Add(-time.Hour+time.Duration(i)*time.Second))
	}
	ip, err := NewInboxPoller(f, einvoiceOnly(InboxConfig{PageSize: 2, Now: c.Now}))
	if err != nil {
		t.Fatal(err)
	}
	sink := &collector{}
	if err := Pipe(context.Background(), ip, sink); err != nil {
		t.Fatal(err)
	}
	if n := len(sink.Events()); n != 7 {
		t.Fatalf("events = %d, want 7", n)
	}
}

func TestInboxPollerAtLeastOnce(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	c := newClock()
	f := newFake(c)
	for i := range 5 {
		deliver(f, provider.EInvoice, fmt.Sprintf("u%d", i), c.Now().Add(-time.Hour+time.Duration(i)*time.Second))
	}
	store := NewMemoryStore()
	cfg := einvoiceOnly(InboxConfig{PageSize: 2, Cursors: store, Dedup: store, Now: c.Now})
	ip, err := NewInboxPoller(f, cfg)
	if err != nil {
		t.Fatal(err)
	}

	// The sink fails on the 4th event (page 2, second item).
	bad := &collector{failAt: 4}
	if err := Pipe(ctx, ip, bad); err == nil {
		t.Fatal("expected sink error")
	}
	if got := bad.UUIDs(); !slices.Equal(got, []string{"u0", "u1", "u2"}) {
		t.Fatalf("delivered before failure = %v", got)
	}
	var cur inboxCursor
	if c, err := ip.loadCursor(ctx, ip.CursorKey(provider.EInvoice)); err != nil {
		t.Fatal(err)
	} else {
		cur = c
	}
	if cur.Pass == nil || cur.Pass.Next != "2" || !cur.Watermark.IsZero() {
		t.Fatalf("cursor after failure = %+v (pass %+v)", cur, cur.Pass)
	}

	// A restarted process with the same store resumes at page 2; u2 is
	// deduplicated, u3 (failed) and u4 are delivered.
	ip2, err := NewInboxPoller(f, cfg)
	if err != nil {
		t.Fatal(err)
	}
	good := &collector{}
	if err := Pipe(ctx, ip2, good); err != nil {
		t.Fatal(err)
	}
	if got := good.UUIDs(); !slices.Equal(got, []string{"u3", "u4"}) {
		t.Fatalf("after restart = %v", got)
	}
	cur, _ = ip2.loadCursor(ctx, ip2.CursorKey(provider.EInvoice))
	if cur.Pass != nil || !cur.Watermark.Equal(c.Now()) {
		t.Fatalf("final cursor = %+v", cur)
	}
}

func TestInboxPollerOnlyNewAck(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	c := newClock()
	f := newFake(c)
	for i := range 5 {
		deliver(f, provider.EInvoice, fmt.Sprintf("u%d", i), c.Now().Add(-time.Hour+time.Duration(i)*time.Second))
	}
	ip, err := NewInboxPoller(f, einvoiceOnly(InboxConfig{PageSize: 2, OnlyNew: true, AckAfterPublish: true, Now: c.Now}))
	if err != nil {
		t.Fatal(err)
	}
	sink := &collector{}
	if err := Pipe(ctx, ip, sink); err != nil {
		t.Fatal(err)
	}
	if n := len(sink.Events()); n != 5 {
		t.Fatalf("events = %d (%v), want 5", n, sink.UUIDs())
	}
	page, err := f.ListInbox(ctx, provider.ListQuery{OnlyNew: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 0 {
		t.Fatalf("not acknowledged: %+v", page.Items)
	}
}

func TestInboxPollerAckFailureKeepsCursor(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	c := newClock()
	f := newFake(c)
	deliver(f, provider.EInvoice, "u0", c.Now().Add(-time.Minute))
	ip, err := NewInboxPoller(f, einvoiceOnly(InboxConfig{AckAfterPublish: true, Now: c.Now}))
	if err != nil {
		t.Fatal(err)
	}
	sink := &collector{failAt: 1}
	if err := Pipe(ctx, ip, sink); err == nil {
		t.Fatal("expected error")
	}
	page, _ := f.ListInbox(ctx, provider.ListQuery{OnlyNew: true})
	if len(page.Items) != 1 {
		t.Fatal("document acknowledged although the sink failed")
	}
	if err := Pipe(ctx, ip, sink); err != nil {
		t.Fatal(err)
	}
	if got := sink.UUIDs(); !slices.Equal(got, []string{"u0"}) {
		t.Fatalf("retry = %v", got)
	}
}

func TestInboxPollerKinds(t *testing.T) {
	t.Parallel()

	c := newClock()
	f := newFake(c)
	deliver(f, provider.EDespatch, "d1", c.Now().Add(-time.Minute))
	deliver(f, provider.EInvoice, "i1", c.Now().Add(-time.Minute))
	ip, err := NewInboxPoller(f, InboxConfig{Now: c.Now})
	if err != nil {
		t.Fatal(err)
	}
	if got := ip.Kinds(); len(got) != 3 {
		t.Fatalf("kinds = %v", got)
	}
	sink := &collector{}
	if err := Pipe(context.Background(), ip, sink); err != nil {
		t.Fatal(err)
	}
	types := map[string]EventType{}
	for _, ev := range sink.Events() {
		types[ev.UUID] = ev.Type
	}
	if types["i1"] != EventInvoiceReceived || types["d1"] != EventDocumentReceived || len(types) != 2 {
		t.Fatalf("types = %v", types)
	}

	none := capsProvider{Fake: f, caps: provider.Capabilities{Operations: map[provider.DocumentKind][]provider.Operation{
		provider.EInvoice: {provider.OpSend},
	}}}
	if _, err := NewInboxPoller(none, InboxConfig{}); err == nil {
		t.Fatal("poller without list-inbox capability")
	}
	if _, err := NewInboxPoller(nil, InboxConfig{}); err == nil {
		t.Fatal("nil provider accepted")
	}
}

func TestInboxPollerListError(t *testing.T) {
	t.Parallel()

	c := newClock()
	f := newFake(c)
	deliver(f, provider.EInvoice, "u0", c.Now())
	p := failingLister{capsProvider{Fake: f, caps: f.Capabilities()}}
	ip, err := NewInboxPoller(p, einvoiceOnly(InboxConfig{Now: c.Now}))
	if err != nil {
		t.Fatal(err)
	}
	err = Pipe(context.Background(), ip, &collector{})
	if !provider.IsRetryable(err) {
		t.Fatalf("err = %v, want retryable", err)
	}
}

type failingLister struct{ capsProvider }

func (failingLister) ListInbox(context.Context, provider.ListQuery) (*provider.Page, error) {
	return nil, &provider.Error{Kind: provider.KindRateLimit, Provider: "fake"}
}
