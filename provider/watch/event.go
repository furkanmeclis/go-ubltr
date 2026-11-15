package watch

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/furkanmeclis/go-ubltr/provider"
)

// EventType names what happened. Values are stable and safe to match on.
type EventType string

// Event types.
const (
	// EventInvoiceReceived is an incoming e-Fatura or e-Arşiv invoice.
	EventInvoiceReceived EventType = "invoice.received"
	// EventDocumentReceived is an incoming document of another kind
	// (e-İrsaliye, receipt advice, application response).
	EventDocumentReceived EventType = "document.received"
	// EventStatusChanged is a new normalized status of an outgoing
	// document.
	EventStatusChanged EventType = "document.status_changed"
	// EventResponseReceived is a commercial KABUL or RED for an outgoing
	// TICARIFATURA invoice. It replaces EventStatusChanged for that
	// transition.
	EventResponseReceived EventType = "response.received"
	// EventEArchiveCancelled is the cancellation of an outgoing e-Arşiv
	// invoice. It replaces EventStatusChanged for that transition.
	EventEArchiveCancelled EventType = "earchive.cancelled"
)

// ProfileCommercial is the cbc:ProfileID of commercial invoices, which
// the receiver answers with KABUL or RED.
const ProfileCommercial = "TICARIFATURA"

// Event is one observed change. The JSON field names are part of the
// public contract.
type Event struct {
	// ID is the idempotency key: provider|tenant|direction|uuid|status.
	// The same document reaching the same status always has the same ID.
	ID   string    `json:"id"`
	Type EventType `json:"type"`
	// Provider is the integrator name (provider.Provider.Name).
	Provider string `json:"provider"`
	// Tenant identifies the account when one process watches several.
	Tenant    string                `json:"tenant,omitempty"`
	Kind      provider.DocumentKind `json:"kind"`
	Direction provider.Direction    `json:"direction"`
	// UUID is the ETTN.
	UUID string `json:"uuid"`
	// DocumentID is the fatura / irsaliye numarası when known.
	DocumentID string          `json:"document_id,omitempty"`
	Status     provider.Status `json:"status"`
	// PreviousStatus is the last status seen before this event, for
	// status changes.
	PreviousStatus provider.Status `json:"previous_status,omitempty"`
	GIBCode        int             `json:"gib_code,omitempty"`
	ProviderCode   string          `json:"provider_code,omitempty"`
	// ResponseCode is KABUL or RED for commercial answers, when the
	// integrator reports it.
	ResponseCode string `json:"response_code,omitempty"`
	// Message is the integrator's status message, when any.
	Message string `json:"message,omitempty"`
	// Info is the listing metadata, when the event came from a listing.
	Info provider.DocumentInfo `json:"info,omitzero"`
	// At is when the change happened as reported by the integrator, or
	// else when the watcher observed it.
	At time.Time `json:"at"`
}

// EventID builds the idempotency key of an event.
func EventID(providerName, tenant string, d provider.Direction, uuid string, s provider.Status) string {
	return strings.Join([]string{providerName, tenant, string(d), uuid, string(s)}, "|")
}

// receivedEvent builds the event for an incoming listing item.
func receivedEvent(providerName, tenant string, info provider.DocumentInfo, at time.Time) Event {
	info.Direction = provider.Inbound
	if info.Status == "" {
		info.Status = provider.StatusReceived
	}
	typ := EventDocumentReceived
	if info.Kind == provider.EInvoice || info.Kind == provider.EArchive {
		typ = EventInvoiceReceived
	}
	return Event{
		ID:           EventID(providerName, tenant, provider.Inbound, info.UUID, info.Status),
		Type:         typ,
		Provider:     providerName,
		Tenant:       tenant,
		Kind:         info.Kind,
		Direction:    provider.Inbound,
		UUID:         info.UUID,
		DocumentID:   info.ID,
		Status:       info.Status,
		GIBCode:      info.GIBCode,
		ProviderCode: info.ProviderCode,
		Info:         info,
		At:           at,
	}
}

// infoJSON is provider.DocumentInfo with snake_case JSON names, so the
// whole event uses one naming style. The field list must match
// provider.DocumentInfo exactly (the conversions below enforce it).
type infoJSON struct {
	Kind         provider.DocumentKind `json:"kind,omitempty"`
	Direction    provider.Direction    `json:"direction,omitempty"`
	UUID         string                `json:"uuid,omitempty"`
	ID           string                `json:"id,omitempty"`
	Profile      string                `json:"profile,omitempty"`
	Type         string                `json:"type,omitempty"`
	IssueDate    string                `json:"issue_date,omitempty"`
	SenderVKN    string                `json:"sender_vkn,omitempty"`
	SenderName   string                `json:"sender_name,omitempty"`
	ReceiverVKN  string                `json:"receiver_vkn,omitempty"`
	ReceiverName string                `json:"receiver_name,omitempty"`
	Currency     string                `json:"currency,omitempty"`
	Payable      string                `json:"payable,omitempty"`
	Status       provider.Status       `json:"status,omitempty"`
	GIBCode      int                   `json:"gib_code,omitempty"`
	ProviderCode string                `json:"provider_code,omitempty"`
	CreatedAt    time.Time             `json:"created_at,omitzero"`
}

type eventFields Event

type eventWire struct {
	eventFields
	Info *infoJSON `json:"info,omitempty"`
}

// MarshalJSON writes the event with snake_case names throughout,
// including the embedded listing metadata.
func (e Event) MarshalJSON() ([]byte, error) {
	w := eventWire{eventFields: eventFields(e)}
	if e.Info != (provider.DocumentInfo{}) {
		info := infoJSON(e.Info)
		w.Info = &info
	}
	return json.Marshal(w)
}

// UnmarshalJSON reads what MarshalJSON writes.
func (e *Event) UnmarshalJSON(b []byte) error {
	var w eventWire
	if err := json.Unmarshal(b, &w); err != nil {
		return err
	}
	*e = Event(w.eventFields)
	if w.Info != nil {
		e.Info = provider.DocumentInfo(*w.Info)
	}
	return nil
}
