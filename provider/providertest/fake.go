// Package providertest offers an in-memory integrator and conformance
// checks for integrator modules.
package providertest

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/furkanmeclis/go-ubltr/provider"
)

// Fake is an in-memory integrator implementing every optional interface.
// It is safe for concurrent use.
type Fake struct {
	mu    sync.Mutex
	caps  provider.Capabilities
	out   map[string]*record
	in    map[string]*record
	seq   int
	now   func() time.Time
	users map[string]provider.UserInfo
	// SendErr, when set, is returned by Send.
	SendErr error
}

type record struct {
	info  provider.DocumentInfo
	xml   []byte
	taken bool
}

// NewFake returns a Fake declaring every operation for every kind.
func NewFake() *Fake {
	ops := slices.Clone(provider.AllOperations)
	caps := provider.Capabilities{
		Operations: map[provider.DocumentKind][]provider.Operation{
			provider.EInvoice:  ops,
			provider.EArchive:  ops,
			provider.EDespatch: ops,
		},
		Formats:       []provider.Format{provider.FormatUBL},
		Protocol:      "memory",
		AcceptsRawUBL: true,
		TestEndpoint:  true,
	}
	return &Fake{caps: caps, out: map[string]*record{}, in: map[string]*record{}, now: time.Now, users: map[string]provider.UserInfo{}}
}

// Name implements provider.Provider.
func (f *Fake) Name() string { return "fake" }

// Capabilities implements provider.Provider.
func (f *Fake) Capabilities() provider.Capabilities { return f.caps }

// SetClock replaces time.Now.
func (f *Fake) SetClock(now func() time.Time) { f.mu.Lock(); f.now = now; f.mu.Unlock() }

// AddUser registers a recipient for CheckUser.
func (f *Fake) AddUser(u provider.UserInfo) { f.mu.Lock(); f.users[u.VKNTCKN] = u; f.mu.Unlock() }

// Send implements provider.Sender.
func (f *Fake) Send(_ context.Context, doc *provider.Document) (*provider.SendResult, error) {
	if f.SendErr != nil {
		return nil, f.SendErr
	}
	if err := doc.Inspect(); err != nil {
		return nil, &provider.Error{Kind: provider.KindValidation, Provider: "fake", Op: "send", Err: err}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if doc.UUID == "" {
		return nil, &provider.Error{Kind: provider.KindValidation, Provider: "fake", Op: "send", Message: "missing UUID"}
	}
	if _, dup := f.out[doc.UUID]; dup {
		return nil, &provider.Error{Kind: provider.KindDuplicate, Provider: "fake", Op: "send", Code: "1163", GIBCode: 1163}
	}
	f.seq++
	id := doc.ID
	if id == "" {
		id = fmt.Sprintf("FAK%d%09d", f.now().Year(), f.seq)
	}
	f.out[doc.UUID] = &record{
		info: provider.DocumentInfo{
			Kind: doc.Kind, Direction: provider.Outbound, UUID: doc.UUID, ID: id,
			Profile: doc.Profile, Type: doc.Type, ReceiverVKN: doc.ReceiverVKN,
			Status: provider.StatusQueued, GIBCode: 1000, CreatedAt: f.now(),
		},
		xml: append([]byte(nil), doc.XML...),
	}
	return &provider.SendResult{UUID: doc.UUID, ID: id, Status: provider.StatusQueued, GIBCode: 1000}, nil
}

// Advance sets the status of an outgoing document (test helper).
func (f *Fake) Advance(uuid string, s provider.Status, gibCode int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.out[uuid]
	if !ok {
		return &provider.Error{Kind: provider.KindNotFound, Provider: "fake"}
	}
	r.info.Status, r.info.GIBCode = s, gibCode
	return nil
}

// Deliver adds an incoming document (test helper).
func (f *Fake) Deliver(info provider.DocumentInfo, xml []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	info.Direction = provider.Inbound
	if info.Status == "" {
		info.Status = provider.StatusReceived
	}
	if info.CreatedAt.IsZero() {
		info.CreatedAt = f.now()
	}
	f.in[info.UUID] = &record{info: info, xml: append([]byte(nil), xml...)}
}

// Status implements provider.StatusChecker.
func (f *Fake) Status(_ context.Context, q provider.StatusQuery) (*provider.StatusResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.box(q.Direction)[q.UUID]
	if !ok {
		return nil, &provider.Error{Kind: provider.KindNotFound, Provider: "fake", Op: "status"}
	}
	return &provider.StatusResult{UUID: r.info.UUID, ID: r.info.ID, Status: r.info.Status, GIBCode: r.info.GIBCode, ProviderCode: strconv.Itoa(r.info.GIBCode), UpdatedAt: f.now()}, nil
}

func (f *Fake) box(d provider.Direction) map[string]*record {
	if d == provider.Inbound {
		return f.in
	}
	return f.out
}

// ListInbox implements provider.InboxLister. Cursor is an offset.
func (f *Fake) ListInbox(_ context.Context, q provider.ListQuery) (*provider.Page, error) {
	return f.list(provider.Inbound, q)
}

// ListOutbox implements provider.OutboxLister.
func (f *Fake) ListOutbox(_ context.Context, q provider.ListQuery) (*provider.Page, error) {
	return f.list(provider.Outbound, q)
}

func (f *Fake) list(d provider.Direction, q provider.ListQuery) (*provider.Page, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var items []provider.DocumentInfo
	for _, r := range f.box(d) {
		if q.Kind != "" && r.info.Kind != q.Kind {
			continue
		}
		if q.OnlyNew && r.taken {
			continue
		}
		if !q.From.IsZero() && r.info.CreatedAt.Before(q.From) {
			continue
		}
		if !q.To.IsZero() && r.info.CreatedAt.After(q.To) {
			continue
		}
		items = append(items, r.info)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].UUID < items[j].UUID
		}
		return items[i].CreatedAt.Before(items[j].CreatedAt)
	})
	off := 0
	if q.Cursor != "" {
		n, err := strconv.Atoi(q.Cursor)
		if err != nil || n < 0 {
			return nil, &provider.Error{Kind: provider.KindInvalidRequest, Provider: "fake", Message: "bad cursor"}
		}
		off = n
	}
	if off > len(items) {
		off = len(items)
	}
	items = items[off:]
	page := &provider.Page{}
	if q.Limit > 0 && len(items) > q.Limit {
		items = items[:q.Limit]
		page.Next = strconv.Itoa(off + q.Limit)
	}
	page.Items = items
	return page, nil
}

// Fetch implements provider.Fetcher (UBL only).
func (f *Fake) Fetch(_ context.Context, q provider.FetchQuery) (*provider.FetchResult, error) {
	if q.Format != "" && q.Format != provider.FormatUBL {
		return nil, provider.Unsupported("fake", provider.OpFetch, q.Kind)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.box(q.Direction)[q.UUID]
	if !ok {
		return nil, &provider.Error{Kind: provider.KindNotFound, Provider: "fake", Op: "fetch"}
	}
	return &provider.FetchResult{Format: provider.FormatUBL, ContentType: "application/xml", Data: append([]byte(nil), r.xml...)}, nil
}

// Respond implements provider.Responder.
func (f *Fake) Respond(_ context.Context, r provider.Response) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	rec, ok := f.in[r.UUID]
	if !ok {
		return &provider.Error{Kind: provider.KindNotFound, Provider: "fake", Op: "respond"}
	}
	if rec.info.Status == provider.StatusAccepted || rec.info.Status == provider.StatusRejected {
		return &provider.Error{Kind: provider.KindInvalidState, Provider: "fake", Op: "respond", Message: "already answered"}
	}
	if r.Accept {
		rec.info.Status = provider.StatusAccepted
	} else {
		rec.info.Status = provider.StatusRejected
	}
	return nil
}

// CancelEArchive implements provider.EArchiveCanceller.
func (f *Fake) CancelEArchive(_ context.Context, uuid, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.out[uuid]
	if !ok {
		return &provider.Error{Kind: provider.KindNotFound, Provider: "fake", Op: "cancel-earchive"}
	}
	if r.info.Kind != provider.EArchive {
		return &provider.Error{Kind: provider.KindInvalidState, Provider: "fake", Op: "cancel-earchive", Message: "not an e-Arşiv invoice"}
	}
	r.info.Status = provider.StatusCancelled
	return nil
}

// CheckUser implements provider.UserChecker.
func (f *Fake) CheckUser(_ context.Context, vkn string) (*provider.UserInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if u, ok := f.users[vkn]; ok {
		return &u, nil
	}
	return &provider.UserInfo{VKNTCKN: vkn}, nil
}

// MarkTaken implements provider.Acknowledger.
func (f *Fake) MarkTaken(_ context.Context, d provider.Direction, uuids ...string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, u := range uuids {
		if r, ok := f.box(d)[u]; ok {
			r.taken = true
		}
	}
	return nil
}

// Credits implements provider.CreditChecker.
func (f *Fake) Credits(context.Context) (*provider.CreditInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return &provider.CreditInfo{Remaining: 1000 - int64(len(f.out)), Used: int64(len(f.out))}, nil
}

var (
	_ provider.Provider          = (*Fake)(nil)
	_ provider.Sender            = (*Fake)(nil)
	_ provider.StatusChecker     = (*Fake)(nil)
	_ provider.InboxLister       = (*Fake)(nil)
	_ provider.OutboxLister      = (*Fake)(nil)
	_ provider.Fetcher           = (*Fake)(nil)
	_ provider.Responder         = (*Fake)(nil)
	_ provider.EArchiveCanceller = (*Fake)(nil)
	_ provider.UserChecker       = (*Fake)(nil)
	_ provider.Acknowledger      = (*Fake)(nil)
	_ provider.CreditChecker     = (*Fake)(nil)
)
