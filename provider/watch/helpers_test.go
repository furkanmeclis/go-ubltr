package watch

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/furkanmeclis/go-ubltr/provider"
	"github.com/furkanmeclis/go-ubltr/provider/providertest"
)

// clock is a settable time source.
type clock struct {
	mu sync.Mutex
	t  time.Time
}

func newClock() *clock { return &clock{t: time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)} }

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *clock) Add(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

// collector is a Sink recording events, optionally failing.
type collector struct {
	mu     sync.Mutex
	events []Event
	// failAt fails the n-th publish (1-based) when > 0.
	failAt int
	calls  int
}

func (c *collector) Publish(_ context.Context, ev Event) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls++
	if c.failAt > 0 && c.calls == c.failAt {
		return fmt.Errorf("sink down")
	}
	c.events = append(c.events, ev)
	return nil
}

func (c *collector) Events() []Event {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]Event(nil), c.events...)
}

func (c *collector) UUIDs() []string {
	var out []string
	for _, ev := range c.Events() {
		out = append(out, ev.UUID)
	}
	return out
}

// capsProvider overrides the Fake's capabilities.
type capsProvider struct {
	*providertest.Fake
	caps provider.Capabilities
}

func (c capsProvider) Capabilities() provider.Capabilities { return c.caps }

func newFake(c *clock) *providertest.Fake {
	f := providertest.NewFake()
	if c != nil {
		f.SetClock(c.Now)
	}
	return f
}

func deliver(f *providertest.Fake, kind provider.DocumentKind, uuid string, at time.Time) {
	f.Deliver(provider.DocumentInfo{Kind: kind, UUID: uuid, ID: "ABC" + uuid, CreatedAt: at}, nil)
}

func invoiceXML(uuid, profile string) []byte {
	return []byte(`<Invoice xmlns="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2" xmlns:cbc="urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2">` +
		`<cbc:ProfileID>` + profile + `</cbc:ProfileID><cbc:ID>ABC2026000000001</cbc:ID><cbc:UUID>` + uuid + `</cbc:UUID>` +
		`<cbc:InvoiceTypeCode>SATIS</cbc:InvoiceTypeCode></Invoice>`)
}

func send(t *testing.T, f *providertest.Fake, uuid, profile string) (*provider.Document, *provider.SendResult) {
	t.Helper()
	doc := &provider.Document{XML: invoiceXML(uuid, profile)}
	res, err := f.Send(context.Background(), doc)
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	return doc, res
}
