package watch

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/furkanmeclis/go-ubltr/provider"
)

// Source produces events. Pollers perform one pass per Run call (the
// [Runner] calls them again on schedule); stream sources such as
// [ReadEvents] run until their input ends. Run stops at the first emit
// error and returns it, so a failing sink never loses an event.
type Source interface {
	Run(ctx context.Context, emit func(Event) error) error
}

// SourceFunc adapts a function to [Source].
type SourceFunc func(ctx context.Context, emit func(Event) error) error

// Run implements Source.
func (f SourceFunc) Run(ctx context.Context, emit func(Event) error) error { return f(ctx, emit) }

// Sink consumes events. Publish returns nil only once the event is safely
// handed off; any error makes the source retry the event later. Sinks
// should treat [Event.ID] as an idempotency key, since delivery is at
// least once.
type Sink interface {
	Publish(ctx context.Context, ev Event) error
}

// CursorStore keeps poller progress by key. Load returns "" for a key that
// was never saved.
type CursorStore interface {
	Load(ctx context.Context, key string) (string, error)
	Save(ctx context.Context, key, value string) error
}

// DedupStore remembers event IDs for a while.
type DedupStore interface {
	// Seen atomically checks and records id. It returns true when id was
	// already recorded and has not expired; otherwise it records id for
	// ttl and returns false.
	Seen(ctx context.Context, id string, ttl time.Duration) (bool, error)
	// Forget removes id, so an event whose delivery failed is not
	// suppressed when it is retried.
	Forget(ctx context.Context, id string) error
}

// Locker grants exclusive leases. Lock returns [ErrLocked] when another
// holder has key. The lease expires after ttl even if unlock is never
// called, so ttl must exceed the longest expected poll pass.
type Locker interface {
	Lock(ctx context.Context, key string, ttl time.Duration) (unlock func(context.Context) error, err error)
}

// ErrLocked is returned by [Locker.Lock] when the key is held elsewhere.
var ErrLocked = errors.New("watch: locked by another holder")

// WatchItem is an outgoing document awaiting a settled status.
type WatchItem struct {
	UUID       string                `json:"uuid"`
	Kind       provider.DocumentKind `json:"kind"`
	DocumentID string                `json:"document_id,omitempty"`
	// Profile is cbc:ProfileID; [ProfileCommercial] keeps the document
	// tracked after delivery until KABUL / RED or the response window
	// ends.
	Profile string `json:"profile,omitempty"`
	// Status is the last status emitted (or reported by Send).
	Status provider.Status `json:"status,omitempty"`
	// AddedAt is when tracking started.
	AddedAt time.Time `json:"added_at"`
	// DeliveredAt is when the document was first seen delivered.
	DeliveredAt time.Time `json:"delivered_at,omitzero"`
}

// WatchList holds outgoing documents per key (usually provider|tenant).
// Add inserts or replaces the item with the same UUID.
type WatchList interface {
	Add(ctx context.Context, key string, item WatchItem) error
	Remove(ctx context.Context, key, uuid string) error
	List(ctx context.Context, key string) ([]WatchItem, error)
}

// Pipe runs src once and publishes every event to sink.
func Pipe(ctx context.Context, src Source, sink Sink) error {
	return src.Run(ctx, func(ev Event) error { return sink.Publish(ctx, ev) })
}

// scopeKey joins provider and tenant into a store key prefix.
func scopeKey(parts ...string) string { return strings.Join(parts, "|") }
