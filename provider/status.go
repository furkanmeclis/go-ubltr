package provider

// Status is a normalized document status. Integrator modules map their own
// states and GİB envelope codes onto it; the raw values stay available on
// the result types.
type Status string

// Statuses.
const (
	StatusUnknown    Status = "unknown"
	StatusDraft      Status = "draft"      // saved, not queued
	StatusQueued     Status = "queued"     // accepted by the integrator
	StatusProcessing Status = "processing" // being signed / enveloped / sent
	StatusSent       Status = "sent"       // delivered to GİB, waiting for the receiver
	StatusDelivered  Status = "delivered"  // receiver system confirmed (GİB 1300)
	StatusAccepted   Status = "accepted"   // commercial KABUL
	StatusRejected   Status = "rejected"   // commercial RED or GİB/receiver refusal
	StatusFailed     Status = "failed"     // technical failure; may be resent
	StatusCancelled  Status = "cancelled"  // e-Arşiv iptal or integrator cancel
	StatusReceived   Status = "received"   // incoming document available
)

// Terminal reports whether no further transitions are expected.
// StatusDelivered is not terminal for TICARIFATURA, which may still be
// accepted or rejected; callers that do not care use Settled.
func (s Status) Terminal() bool {
	switch s {
	case StatusAccepted, StatusRejected, StatusFailed, StatusCancelled:
		return true
	}
	return false
}

// Settled reports whether the document reached the receiver or ended.
func (s Status) Settled() bool {
	return s == StatusDelivered || s.Terminal()
}

// Valid reports whether s is one of the declared statuses.
func (s Status) Valid() bool {
	switch s {
	case StatusUnknown, StatusDraft, StatusQueued, StatusProcessing, StatusSent,
		StatusDelivered, StatusAccepted, StatusRejected, StatusFailed,
		StatusCancelled, StatusReceived:
		return true
	}
	return false
}
