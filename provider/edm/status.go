package edm

import (
	"strings"

	"github.com/furkanmeclis/go-ubltr/provider"
)

// normalizeWords turns "SEND - SUCCEED" into "SEND-SUCCEED", the key form
// used in errors.csv.
func normalizeWords(s string) string {
	return strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(s), " ", ""))
}

// documentStatus maps the STATUS text through the code table and combines
// it with GIB_STATUS_CODE; a failing GİB code wins over an in-progress
// vendor status.
func documentStatus(status string, gibCode int) provider.Status {
	s := provider.StatusUnknown
	if c, ok := Codes.Lookup(normalizeWords(status)); ok {
		s = c.Status
	}
	g := provider.GIBStatus(gibCode)
	switch {
	case s == provider.StatusUnknown:
		s = g
	case g == provider.StatusFailed && !s.Terminal():
		s = g
	}
	return s
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
