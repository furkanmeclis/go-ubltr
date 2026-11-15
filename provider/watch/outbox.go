package watch

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/furkanmeclis/go-ubltr/provider"
)

// Outbox tracker defaults.
const (
	// DefaultResponseWindow is how long a delivered TICARIFATURA invoice
	// stays tracked waiting for KABUL / RED (8 days, the commercial
	// objection period).
	DefaultResponseWindow = 8 * 24 * time.Hour
	// DefaultMaxAge drops documents that never settle.
	DefaultMaxAge = 30 * 24 * time.Hour
)

// OutboxConfig configures an [OutboxTracker]. Zero values select defaults.
type OutboxConfig struct {
	// Tenant identifies the account in events and the watch-list key.
	Tenant string
	// WatchList holds the tracked documents. Default: a new memory store.
	WatchList WatchList
	// ResponseWindow bounds how long a delivered TICARIFATURA invoice is
	// tracked for a commercial answer (default 8 days from delivery).
	ResponseWindow time.Duration
	// MaxAge drops any document tracked for longer (default 30 days).
	MaxAge time.Duration
	// WaitTerminal keeps non-commercial documents tracked until a
	// terminal status (for example an e-Arşiv cancellation) instead of
	// stopping once they are delivered.
	WaitTerminal bool
	// Now replaces time.Now (tests).
	Now func() time.Time
}

// OutboxTracker follows outgoing documents on a [WatchList]. Each Run
// asks provider.StatusChecker for every tracked document, emits an event
// when the status changed and stops tracking once the status is settled:
// provider.Status.Terminal for TICARIFATURA invoices (or after the
// response window), provider.Status.Settled for the rest unless
// WaitTerminal is set.
//
// A KABUL / RED is emitted as [EventResponseReceived], an e-Arşiv
// cancellation as [EventEArchiveCancelled] and every other change as
// [EventStatusChanged].
type OutboxTracker struct {
	name    string
	checker provider.StatusChecker
	cfg     OutboxConfig
}

// NewOutboxTracker checks that p implements provider.StatusChecker and
// fills defaults.
func NewOutboxTracker(p provider.Provider, cfg OutboxConfig) (*OutboxTracker, error) {
	if p == nil {
		return nil, errors.New("watch: nil provider")
	}
	sc, ok := p.(provider.StatusChecker)
	if !ok {
		return nil, provider.Unsupported(p.Name(), provider.OpStatus, "")
	}
	if cfg.WatchList == nil {
		cfg.WatchList = NewMemoryStore()
	}
	if cfg.ResponseWindow <= 0 {
		cfg.ResponseWindow = DefaultResponseWindow
	}
	if cfg.MaxAge <= 0 {
		cfg.MaxAge = DefaultMaxAge
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &OutboxTracker{name: p.Name(), checker: sc, cfg: cfg}, nil
}

// Key is the WatchList key of this tracker.
func (t *OutboxTracker) Key() string { return scopeKey("outbox", t.name, t.cfg.Tenant) }

// Track starts following a document. Kind defaults to e-Fatura, Status to
// queued and AddedAt to now.
func (t *OutboxTracker) Track(ctx context.Context, item WatchItem) error {
	if item.UUID == "" {
		return errors.New("watch: track without UUID")
	}
	if item.Kind == "" {
		item.Kind = provider.EInvoice
	}
	if item.Status == "" {
		item.Status = provider.StatusQueued
	}
	now := t.cfg.Now()
	if item.AddedAt.IsZero() {
		item.AddedAt = now
	}
	if item.Status == provider.StatusDelivered && item.DeliveredAt.IsZero() {
		item.DeliveredAt = now
	}
	return t.cfg.WatchList.Add(ctx, t.Key(), item)
}

// TrackSent follows a document right after provider.Sender.Send.
func (t *OutboxTracker) TrackSent(ctx context.Context, doc *provider.Document, res *provider.SendResult) error {
	item := WatchItem{}
	if doc != nil {
		item.UUID, item.Kind, item.DocumentID, item.Profile = doc.UUID, doc.Kind, doc.ID, doc.Profile
	}
	if res != nil {
		if res.UUID != "" {
			item.UUID = res.UUID
		}
		if res.ID != "" {
			item.DocumentID = res.ID
		}
		item.Status = res.Status
	}
	return t.Track(ctx, item)
}

// Untrack stops following uuid.
func (t *OutboxTracker) Untrack(ctx context.Context, uuid string) error {
	return t.cfg.WatchList.Remove(ctx, t.Key(), uuid)
}

// Run implements Source: one status check per tracked document.
// Retryable errors stop the pass; other per-document errors are collected
// and returned after the remaining documents were checked.
func (t *OutboxTracker) Run(ctx context.Context, emit func(Event) error) error {
	key := t.Key()
	items, err := t.cfg.WatchList.List(ctx, key)
	if err != nil {
		return err
	}
	var errs []error
	for _, item := range items {
		if err := ctx.Err(); err != nil {
			return errors.Join(append(errs, err)...)
		}
		changed := false
		res, err := t.checker.Status(ctx, provider.StatusQuery{Kind: item.Kind, Direction: provider.Outbound, UUID: item.UUID})
		switch {
		case err == nil:
			if res.Status != "" && res.Status != provider.StatusUnknown && res.Status != item.Status {
				if err := emit(t.statusEvent(item, res)); err != nil {
					return errors.Join(append(errs, err)...)
				}
				item.Status = res.Status
				if res.ID != "" {
					item.DocumentID = res.ID
				}
				if res.ResponseCode != "" && item.Profile == "" {
					item.Profile = ProfileCommercial
				}
				changed = true
			}
		case provider.IsRetryable(err):
			return errors.Join(append(errs, err)...)
		case errors.Is(err, provider.ErrNotFound):
			// Not indexed yet; MaxAge eventually drops it.
		default:
			errs = append(errs, fmt.Errorf("watch: status %s: %w", item.UUID, err))
			continue
		}
		now := t.cfg.Now()
		if item.Status == provider.StatusDelivered && item.DeliveredAt.IsZero() {
			item.DeliveredAt = now
			changed = true
		}
		switch {
		case t.done(item, now):
			err = t.cfg.WatchList.Remove(ctx, key, item.UUID)
		case changed:
			err = t.cfg.WatchList.Add(ctx, key, item)
		default:
			err = nil
		}
		if err != nil {
			return errors.Join(append(errs, err)...)
		}
	}
	return errors.Join(errs...)
}

func commercial(item WatchItem) bool {
	return item.Kind == provider.EInvoice && item.Profile == ProfileCommercial
}

func (t *OutboxTracker) done(item WatchItem, now time.Time) bool {
	switch {
	case item.Status.Terminal():
		return true
	case commercial(item):
		if item.Status == provider.StatusDelivered && now.Sub(item.DeliveredAt) >= t.cfg.ResponseWindow {
			return true
		}
	case !t.cfg.WaitTerminal && item.Status.Settled():
		return true
	}
	return now.Sub(item.AddedAt) >= t.cfg.MaxAge
}

func (t *OutboxTracker) statusEvent(item WatchItem, res *provider.StatusResult) Event {
	typ := EventStatusChanged
	answered := res.Status == provider.StatusAccepted || res.Status == provider.StatusRejected
	switch {
	case answered && (commercial(item) || res.ResponseCode != ""):
		typ = EventResponseReceived
	case item.Kind == provider.EArchive && res.Status == provider.StatusCancelled:
		typ = EventEArchiveCancelled
	}
	id := item.DocumentID
	if res.ID != "" {
		id = res.ID
	}
	at := res.UpdatedAt
	if at.IsZero() {
		at = t.cfg.Now()
	}
	return Event{
		ID:             EventID(t.name, t.cfg.Tenant, provider.Outbound, item.UUID, res.Status),
		Type:           typ,
		Provider:       t.name,
		Tenant:         t.cfg.Tenant,
		Kind:           item.Kind,
		Direction:      provider.Outbound,
		UUID:           item.UUID,
		DocumentID:     id,
		Status:         res.Status,
		PreviousStatus: item.Status,
		GIBCode:        res.GIBCode,
		ProviderCode:   res.ProviderCode,
		ResponseCode:   res.ResponseCode,
		Message:        res.Message,
		At:             at,
	}
}

var _ Source = (*OutboxTracker)(nil)
