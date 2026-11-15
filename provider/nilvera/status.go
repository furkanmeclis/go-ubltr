package nilvera

import (
	"strings"

	"github.com/furkanmeclis/go-ubltr/provider"
)

// InvoiceStatus maps the e-Fatura StatusCode enum (unknown, waiting,
// succeed, error) together with the envelope GİB code. A known GİB code
// refines a non-error status (1000 queued, 1200/1220 sent, 1300
// delivered); a failing GİB code wins over "succeed".
func InvoiceStatus(code string, gibCode int) provider.Status {
	var s provider.Status
	switch strings.ToLower(code) {
	case "waiting":
		s = provider.StatusProcessing
	case "succeed":
		s = provider.StatusSent
	case "error":
		return provider.StatusFailed
	default:
		s = provider.StatusUnknown
	}
	if gibCode != 0 {
		if g := provider.GIBStatus(gibCode); g != provider.StatusUnknown {
			return g
		}
	}
	return s
}

// ArchiveStatus maps the e-Arşiv StatusCode enum with ReportStatus
// (NotReported, Reported) and the cancel flag.
func ArchiveStatus(code, report string, cancelled bool) provider.Status {
	if cancelled {
		return provider.StatusCancelled
	}
	switch strings.ToLower(code) {
	case "waiting":
		return provider.StatusProcessing
	case "succeed":
		if report == "Reported" {
			return provider.StatusDelivered
		}
		return provider.StatusSent
	case "error":
		return provider.StatusFailed
	}
	return provider.StatusUnknown
}

// answerStatus maps the AnswerCode enum. documentAnsweredAutomatically is
// treated as acceptance (error 1003 calls it "otomatik olarak kabul edildi").
func answerStatus(code string) (provider.Status, string, bool) {
	switch strings.ToLower(code) {
	case "approved", "documentansweredautomatically":
		return provider.StatusAccepted, "KABUL", true
	case "rejected":
		return provider.StatusRejected, "RED", true
	}
	return "", "", false
}
