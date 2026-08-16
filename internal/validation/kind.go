package validation

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
)

// Kind is the document root (local name + namespace), not a Schematron profile.
// KindEArchiveReport is the GİB e-Arşiv *report* (eArsivRaporu), not an
// e-Arşiv Invoice (UBL Invoice with ProfileID EARSIVFATURA).
type Kind string

const (
	KindInvoice             Kind = "invoice"
	KindCreditNote          Kind = "creditnote"
	KindApplicationResponse Kind = "applicationresponse"
	KindDespatchAdvice      Kind = "despatchadvice"
	KindReceiptAdvice       Kind = "receiptadvice"
	KindEArchiveReport      Kind = "earchivereport"
)

const (
	nsInvoice             = "urn:oasis:names:specification:ubl:schema:xsd:Invoice-2"
	nsCreditNote          = "urn:oasis:names:specification:ubl:schema:xsd:CreditNote-2"
	nsApplicationResponse = "urn:oasis:names:specification:ubl:schema:xsd:ApplicationResponse-2"
	nsDespatchAdvice      = "urn:oasis:names:specification:ubl:schema:xsd:DespatchAdvice-2"
	nsReceiptAdvice       = "urn:oasis:names:specification:ubl:schema:xsd:ReceiptAdvice-2"
	nsEArchiveReport      = "http://earsiv.efatura.gov.tr"
)

// ErrUnknownDocument is returned when the XML root is not a supported document.
var ErrUnknownDocument = errors.New("unknown ubl document")

// Detect reports the document kind from the XML root element.
func Detect(data []byte) (Kind, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	for {
		tok, err := dec.Token()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return "", fmt.Errorf("%w: no root element", ErrUnknownDocument)
			}
			return "", fmt.Errorf("detect document: %w", err)
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		return kindFromName(se.Name)
	}
}

func kindFromName(n xml.Name) (Kind, error) {
	switch n.Local {
	case "Invoice":
		if n.Space == "" || n.Space == nsInvoice {
			return KindInvoice, nil
		}
	case "CreditNote":
		if n.Space == "" || n.Space == nsCreditNote {
			return KindCreditNote, nil
		}
	case "ApplicationResponse":
		if n.Space == "" || n.Space == nsApplicationResponse {
			return KindApplicationResponse, nil
		}
	case "DespatchAdvice":
		if n.Space == "" || n.Space == nsDespatchAdvice {
			return KindDespatchAdvice, nil
		}
	case "ReceiptAdvice":
		if n.Space == "" || n.Space == nsReceiptAdvice {
			return KindReceiptAdvice, nil
		}
	case "eArsivRaporu":
		if n.Space == "" || n.Space == nsEArchiveReport {
			return KindEArchiveReport, nil
		}
	}
	return "", fmt.Errorf("%w: {%s}%s", ErrUnknownDocument, n.Space, n.Local)
}
