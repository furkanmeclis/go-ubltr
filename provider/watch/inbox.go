package watch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/furkanmeclis/go-ubltr/provider"
)

// Inbox poller defaults.
const (
	DefaultOverlap  = 15 * time.Minute
	DefaultLookback = 24 * time.Hour
	DefaultDedupTTL = 7 * 24 * time.Hour
	DefaultPageSize = 100
)

// InboxConfig configures an [InboxPoller]. Zero values select defaults.
type InboxConfig struct {
	// Tenant identifies the account in events and store keys.
	Tenant string
	// Kinds to poll. Default: every kind declaring provider.OpListInbox.
	Kinds []provider.DocumentKind
	// Cursors keeps the time watermark. Default: a new memory store.
	Cursors CursorStore
	// Dedup suppresses events already delivered, which the overlap window
	// lists again. Default: a new memory store.
	Dedup DedupStore
	// DedupTTL is how long delivered IDs are remembered (default 7 days).
	// It must be longer than Overlap.
	DedupTTL time.Duration
	// Overlap moves each pass's start back from the last watermark, to
	// catch documents the integrator indexed late or stamped with a
	// skewed clock (default 15 minutes).
	Overlap time.Duration
	// Lookback is how far back the very first pass starts (default 24h).
	Lookback time.Duration
	// PageSize is the listing page-size hint (default 100).
	PageSize int
	// OnlyNew asks the integrator for documents not yet marked taken. It
	// takes effect only when the integrator supports provider.OpMarkTaken.
	OnlyNew bool
	// AckAfterPublish calls MarkTaken for every listed document once its
	// page was accepted by the sink, when the integrator supports it.
	AckAfterPublish bool
	// Now replaces time.Now (tests).
	Now func() time.Time
}

// InboxPoller lists incoming documents and emits [EventInvoiceReceived]
// (or [EventDocumentReceived] for non-invoice kinds). Each Run is one
// pass over every kind.
//
// Progress is a time watermark per kind. A pass lists [watermark-Overlap,
// now]; the watermark moves to now only when the whole pass was
// accepted. Within a pass the integrator's page cursor is saved after
// every accepted page, so an interrupted pass resumes where it stopped.
// When OnlyNew and AckAfterPublish are both in effect, the poller instead
// re-lists from the start after acknowledging each page, since the
// acknowledged documents drop out of the listing.
type InboxPoller struct {
	name     string
	lister   provider.InboxLister
	ack      provider.Acknowledger
	cfg      InboxConfig
	kinds    []provider.DocumentKind
	onlyNew  bool
	ackPages bool
}

// NewInboxPoller checks that p implements provider.InboxLister and fills
// defaults.
func NewInboxPoller(p provider.Provider, cfg InboxConfig) (*InboxPoller, error) {
	if p == nil {
		return nil, errors.New("watch: nil provider")
	}
	lister, ok := p.(provider.InboxLister)
	if !ok {
		return nil, provider.Unsupported(p.Name(), provider.OpListInbox, "")
	}
	caps := p.Capabilities()
	ip := &InboxPoller{name: p.Name(), lister: lister, cfg: cfg}
	if len(cfg.Kinds) == 0 {
		for _, k := range caps.Kinds() {
			if caps.Has(k, provider.OpListInbox) {
				ip.kinds = append(ip.kinds, k)
			}
		}
	} else {
		ip.kinds = append(ip.kinds, cfg.Kinds...)
	}
	if len(ip.kinds) == 0 {
		return nil, provider.Unsupported(p.Name(), provider.OpListInbox, "any kind")
	}
	if a, ok := p.(provider.Acknowledger); ok {
		for _, k := range ip.kinds {
			if caps.Has(k, provider.OpMarkTaken) {
				ip.ack = a
				break
			}
		}
	}
	ip.onlyNew = cfg.OnlyNew && ip.ack != nil
	if !cfg.AckAfterPublish {
		ip.ack = nil
	}
	ip.ackPages = ip.onlyNew && ip.ack != nil
	if ip.cfg.Cursors == nil || ip.cfg.Dedup == nil {
		mem := NewMemoryStore()
		if ip.cfg.Cursors == nil {
			ip.cfg.Cursors = mem
		}
		if ip.cfg.Dedup == nil {
			ip.cfg.Dedup = mem
		}
	}
	if ip.cfg.Overlap <= 0 {
		ip.cfg.Overlap = DefaultOverlap
	}
	if ip.cfg.Lookback <= 0 {
		ip.cfg.Lookback = DefaultLookback
	}
	if ip.cfg.DedupTTL <= 0 {
		ip.cfg.DedupTTL = DefaultDedupTTL
	}
	if ip.cfg.DedupTTL < ip.cfg.Overlap {
		ip.cfg.DedupTTL = 2 * ip.cfg.Overlap
	}
	if ip.cfg.PageSize <= 0 {
		ip.cfg.PageSize = DefaultPageSize
	}
	if ip.cfg.Now == nil {
		ip.cfg.Now = time.Now
	}
	return ip, nil
}

// Kinds returns the document kinds this poller lists.
func (ip *InboxPoller) Kinds() []provider.DocumentKind {
	return append([]provider.DocumentKind(nil), ip.kinds...)
}

// CursorKey is the CursorStore key used for kind.
func (ip *InboxPoller) CursorKey(kind provider.DocumentKind) string {
	return scopeKey("inbox", ip.name, ip.cfg.Tenant, string(kind))
}

type inboxCursor struct {
	Watermark time.Time  `json:"watermark,omitzero"`
	Pass      *inboxPass `json:"pass,omitempty"`
}

type inboxPass struct {
	From time.Time `json:"from"`
	To   time.Time `json:"to"`
	Next string    `json:"next,omitempty"`
}

// Run implements Source: one pass over every kind.
func (ip *InboxPoller) Run(ctx context.Context, emit func(Event) error) error {
	for _, k := range ip.kinds {
		if err := ip.pollKind(ctx, k, emit); err != nil {
			return err
		}
	}
	return nil
}

func (ip *InboxPoller) loadCursor(ctx context.Context, key string) (inboxCursor, error) {
	var c inboxCursor
	raw, err := ip.cfg.Cursors.Load(ctx, key)
	if err != nil || raw == "" {
		return c, err
	}
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		return c, fmt.Errorf("watch: cursor %s: %w", key, err)
	}
	return c, nil
}

func (ip *InboxPoller) saveCursor(ctx context.Context, key string, c inboxCursor) error {
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}
	return ip.cfg.Cursors.Save(ctx, key, string(b))
}

func (ip *InboxPoller) pollKind(ctx context.Context, kind provider.DocumentKind, emit func(Event) error) error {
	key := ip.CursorKey(kind)
	cur, err := ip.loadCursor(ctx, key)
	if err != nil {
		return err
	}
	if cur.Pass == nil {
		now := ip.cfg.Now()
		start := cur.Watermark
		if start.IsZero() {
			start = now.Add(-ip.cfg.Lookback)
		}
		cur.Pass = &inboxPass{From: start.Add(-ip.cfg.Overlap), To: now}
	}
	pass := cur.Pass
	handled := map[string]bool{}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		q := provider.ListQuery{Kind: kind, From: pass.From, To: pass.To, OnlyNew: ip.onlyNew, Limit: ip.cfg.PageSize}
		if !ip.ackPages {
			q.Cursor = pass.Next
		}
		page, err := ip.lister.ListInbox(ctx, q)
		if err != nil {
			return err
		}
		fresh, uuids := 0, make([]string, 0, len(page.Items))
		for _, info := range page.Items {
			if info.UUID == "" || handled[info.UUID] {
				continue
			}
			handled[info.UUID] = true
			fresh++
			if info.Kind == "" {
				info.Kind = kind
			}
			ev := receivedEvent(ip.name, ip.cfg.Tenant, info, ip.cfg.Now())
			seen, err := ip.cfg.Dedup.Seen(ctx, ev.ID, ip.cfg.DedupTTL)
			if err != nil {
				return err
			}
			uuids = append(uuids, info.UUID)
			if seen {
				continue
			}
			if err := emit(ev); err != nil {
				if ferr := ip.cfg.Dedup.Forget(context.WithoutCancel(ctx), ev.ID); ferr != nil {
					return errors.Join(err, ferr)
				}
				return err
			}
		}
		if ip.ack != nil && len(uuids) > 0 {
			if err := ip.ack.MarkTaken(ctx, provider.Inbound, uuids...); err != nil {
				return err
			}
		}
		done := page.Next == ""
		if ip.ackPages {
			done = done || fresh == 0
		} else {
			pass.Next = page.Next
		}
		if done {
			break
		}
		if !ip.ackPages {
			if err := ip.saveCursor(ctx, key, cur); err != nil {
				return err
			}
		}
	}
	return ip.saveCursor(ctx, key, inboxCursor{Watermark: pass.To})
}

var _ Source = (*InboxPoller)(nil)
