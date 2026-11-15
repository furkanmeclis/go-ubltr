package provider

import "time"

// Operation names a capability.
type Operation string

// Operations. Each matches one optional interface.
const (
	OpSend           Operation = "send"            // Sender
	OpStatus         Operation = "status"          // StatusChecker
	OpListInbox      Operation = "list-inbox"      // InboxLister
	OpListOutbox     Operation = "list-outbox"     // OutboxLister
	OpFetch          Operation = "fetch"           // Fetcher
	OpRespond        Operation = "respond"         // Responder
	OpCancelEArchive Operation = "cancel-earchive" // EArchiveCanceller
	OpCheckUser      Operation = "check-user"      // UserChecker
	OpMarkTaken      Operation = "mark-taken"      // Acknowledger
	OpCredits        Operation = "credits"         // CreditChecker
)

// AllOperations lists every operation in a stable order.
var AllOperations = []Operation{
	OpSend, OpStatus, OpListInbox, OpListOutbox, OpFetch, OpRespond,
	OpCancelEArchive, OpCheckUser, OpMarkTaken, OpCredits,
}

// Capabilities describes what an integrator module supports.
type Capabilities struct {
	// Operations supported for each document kind.
	Operations map[DocumentKind][]Operation
	// Formats returned by Fetch.
	Formats []Format
	// Protocol is "soap", "rest" or both.
	Protocol string
	// Webhooks is true when the integrator can push events, so pollers are
	// not needed. Push delivery is configured with the integrator.
	Webhooks bool
	// AssignsNumbers is true when the integrator generates cbc:ID itself
	// and Document.ID may be left empty.
	AssignsNumbers bool
	// SignsDocuments is true when the integrator applies the mali mühür.
	SignsDocuments bool
	// AcceptsRawUBL is true when Send takes UBL-TR XML as-is.
	AcceptsRawUBL bool
	// MaxBatch is the largest number of documents per call (0 = unknown).
	MaxBatch int
	// MinPollInterval is the smallest polling interval the integrator
	// tolerates (0 = unknown).
	MinPollInterval time.Duration
	// TestEndpoint is true when a public or on-request test system exists.
	TestEndpoint bool
}

// Has reports whether op is supported for kind.
func (c Capabilities) Has(kind DocumentKind, op Operation) bool {
	for _, o := range c.Operations[kind] {
		if o == op {
			return true
		}
	}
	return false
}

// Kinds lists document kinds with at least one operation.
func (c Capabilities) Kinds() []DocumentKind {
	var out []DocumentKind
	for _, k := range []DocumentKind{EInvoice, EArchive, EDespatch, ReceiptAdvice, ApplicationResponse} {
		if len(c.Operations[k]) > 0 {
			out = append(out, k)
		}
	}
	return out
}

// Supports reports whether p declares op for kind and implements the
// matching interface.
func Supports(p Provider, kind DocumentKind, op Operation) bool {
	if p == nil || !p.Capabilities().Has(kind, op) {
		return false
	}
	var ok bool
	switch op {
	case OpSend:
		_, ok = p.(Sender)
	case OpStatus:
		_, ok = p.(StatusChecker)
	case OpListInbox:
		_, ok = p.(InboxLister)
	case OpListOutbox:
		_, ok = p.(OutboxLister)
	case OpFetch:
		_, ok = p.(Fetcher)
	case OpRespond:
		_, ok = p.(Responder)
	case OpCancelEArchive:
		_, ok = p.(EArchiveCanceller)
	case OpCheckUser:
		_, ok = p.(UserChecker)
	case OpMarkTaken:
		_, ok = p.(Acknowledger)
	case OpCredits:
		_, ok = p.(CreditChecker)
	}
	return ok
}

// Unsupported returns an ErrUnsupported *Error for op.
func Unsupported(providerName string, op Operation, kind DocumentKind) error {
	return &Error{
		Kind:     KindUnsupported,
		Provider: providerName,
		Op:       string(op),
		Message:  "operation not supported for " + string(kind),
	}
}
