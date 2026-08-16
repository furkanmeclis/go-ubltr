package ubltr

import "github.com/furkanmeclis/go-ubltr/internal/validation"

// Kind is the document root (element local name + namespace).
// It is not a Schematron profile (TEMELFATURA, EARSIVFATURA, …).
// KindEArchiveReport is eArsivRaporu, not an e-Arşiv Invoice.
type Kind string

const (
	KindInvoice             Kind = Kind(validation.KindInvoice)
	KindCreditNote          Kind = Kind(validation.KindCreditNote)
	KindApplicationResponse Kind = Kind(validation.KindApplicationResponse)
	KindDespatchAdvice      Kind = Kind(validation.KindDespatchAdvice)
	KindReceiptAdvice       Kind = Kind(validation.KindReceiptAdvice)
	KindEArchiveReport      Kind = Kind(validation.KindEArchiveReport)
)

// ErrUnknownDocument is returned by [Detect] and [ValidateXSD] when the XML
// root is not a supported document.
var ErrUnknownDocument = validation.ErrUnknownDocument

// Detect reports the document kind from the XML root element.
func Detect(data []byte) (Kind, error) {
	k, err := validation.Detect(data)
	if err != nil {
		return "", err
	}
	return Kind(k), nil
}
