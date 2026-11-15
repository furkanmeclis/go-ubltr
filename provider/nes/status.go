package nes

import (
	"strconv"
	"strings"

	"github.com/furkanmeclis/go-ubltr/provider"
)

// OutgoingStatus maps the OutgoingStatus enum of the e-Fatura API.
func OutgoingStatus(s string) provider.Status {
	switch s {
	case "None", "Waiting":
		return provider.StatusQueued
	case "WaitingToBeSigned", "WaitingToBeEnvelopeCreate", "WaitingToBeSend",
		"EnvelopeIsWaitingToBeProcessedByGib":
		return provider.StatusProcessing
	case "EnvelopeIsWaitingToBeSendedToReceiverByGib",
		"EnvelopeIsBeenWaitingToBeTransferredToReceiverByGib",
		"EnvelopeSystemResponseIsBeingWaitedFromReceiver":
		return provider.StatusSent
	case "EnvelopeHasBeenTransferredToReceiverSuccessfully":
		return provider.StatusDelivered
	case "ErrorHasBeenDetectedInEnvelopeByGib", "EnvelopeCouldNotTransferredToReceiverByGib",
		"ErrorHasBeenDetectedInEnvelopeByReceiver", "FileNotFound", "XmlParseError":
		return provider.StatusFailed
	}
	return provider.StatusUnknown
}

// answerStatus maps the DocumentAnswer enum; ok is false for None and
// Waiting.
func answerStatus(s string) (provider.Status, string, bool) {
	switch s {
	case "Accepted":
		return provider.StatusAccepted, "KABUL", true
	case "Rejected":
		return provider.StatusRejected, "RED", true
	}
	return "", "", false
}

// ArchiveStatus maps the e-Arşiv ArchiveDocumentStatus enum together with
// the cancellation flag and report status (EArchiveDocumentsReportDivisionStatus).
func ArchiveStatus(s string, canceled bool, report string) provider.Status {
	if canceled {
		return provider.StatusCancelled
	}
	switch s {
	case "Waiting":
		return provider.StatusQueued
	case "WaitingSign":
		return provider.StatusProcessing
	case "Signed":
		if report == "Completed" {
			return provider.StatusDelivered
		}
		return provider.StatusSent
	case "Error", "XmlFileDoesNotExist", "XmlParseError":
		return provider.StatusFailed
	}
	return provider.StatusUnknown
}

// gibCode returns the envelope code as an int when it is a GİB code.
func gibCode(code string) int {
	n, err := strconv.Atoi(strings.TrimSpace(code))
	if err != nil {
		return 0
	}
	if _, ok := provider.GIBCodes.Lookup(strconv.Itoa(n)); !ok {
		return 0
	}
	return n
}
