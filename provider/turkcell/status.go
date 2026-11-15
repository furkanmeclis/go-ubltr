package turkcell

import (
	"strconv"

	"github.com/furkanmeclis/go-ubltr/provider"
)

// OutboxStatus maps the outgoing e-Fatura / e-Arşiv status code through
// the code table.
func OutboxStatus(code int) provider.Status {
	if c, ok := Codes.Lookup(strconv.Itoa(code)); ok {
		return c.Status
	}
	return provider.StatusUnknown
}

// InboxStatus maps the incoming-invoice status code. Incoming documents
// are received unless the code records a final answer or cancellation.
func InboxStatus(code int) provider.Status {
	switch code {
	case 40:
		return provider.StatusFailed
	case 65:
		return provider.StatusAccepted
	case 80, 85:
		return provider.StatusRejected
	case 99:
		return provider.StatusCancelled
	}
	return provider.StatusReceived
}

func responseCode(s provider.Status) string {
	switch s {
	case provider.StatusAccepted:
		return "KABUL"
	case provider.StatusRejected:
		return "RED"
	}
	return ""
}

// envelopeGIB returns the envelope status as a GİB code when it is one.
func envelopeGIB(code int) int {
	if _, ok := provider.GIBCodes.Lookup(strconv.Itoa(code)); ok {
		return code
	}
	return 0
}
