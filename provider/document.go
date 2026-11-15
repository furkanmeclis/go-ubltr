package provider

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"
)

// DocumentKind is the GİB document family.
type DocumentKind string

// Document kinds.
const (
	EInvoice            DocumentKind = "efatura"
	EArchive            DocumentKind = "earsiv"
	EDespatch           DocumentKind = "eirsaliye"
	ReceiptAdvice       DocumentKind = "eirsaliye-yanit"
	ApplicationResponse DocumentKind = "uygulama-yanit"
)

// Document is an outgoing UBL-TR document.
type Document struct {
	Kind DocumentKind
	// UUID (ETTN) and ID (fatura numarası) as found in the XML. Fill them
	// with Inspect, or leave ID empty when the integrator assigns numbers.
	UUID string
	ID   string
	// Profile and Type are cbc:ProfileID / cbc:InvoiceTypeCode.
	Profile string
	Type    string
	// ReceiverAlias is the recipient posta kutusu for e-Fatura.
	ReceiverAlias string
	// SenderAlias is the sender gönderici birim, when the account has several.
	SenderAlias string
	// ReceiverVKN is the recipient VKN/TCKN.
	ReceiverVKN string
	// XML is the UBL-TR document.
	XML []byte
	// Email optionally asks the integrator to e-mail an e-Arşiv invoice.
	Email string
}

// SendResult is returned by Sender.
type SendResult struct {
	UUID         string
	ID           string
	EnvelopeUUID string
	Status       Status
	GIBCode      int
	ProviderCode string
	Message      string
	Raw          []byte
}

// Inspect fills Kind, UUID, ID, Profile, Type and ReceiverVKN from the XML
// root without validating it. Existing non-empty fields are kept.
func (d *Document) Inspect() error {
	if d == nil || len(d.XML) == 0 {
		return errors.New("provider: empty document")
	}
	info, err := sniff(d.XML)
	if err != nil {
		return err
	}
	set := func(dst *string, v string) {
		if *dst == "" {
			*dst = v
		}
	}
	if d.Kind == "" {
		d.Kind = info.kind
	}
	set(&d.UUID, info.uuid)
	set(&d.ID, info.id)
	set(&d.Profile, info.profile)
	set(&d.Type, info.typ)
	set(&d.ReceiverVKN, info.receiver)
	return nil
}

type sniffed struct {
	kind                             DocumentKind
	uuid, id, profile, typ, receiver string
}

// sniff reads the top-level cbc fields and the customer party identifier.
func sniff(data []byte) (sniffed, error) {
	var s sniffed
	dec := xml.NewDecoder(bytes.NewReader(data))
	depth := 0
	var stack []string
	inCustomer := false
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return s, fmt.Errorf("provider: parse xml: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			stack = append(stack, t.Name.Local)
			if depth == 1 {
				switch t.Name.Local {
				case "Invoice":
					s.kind = EInvoice
				case "DespatchAdvice":
					s.kind = EDespatch
				case "ReceiptAdvice":
					s.kind = ReceiptAdvice
				case "ApplicationResponse":
					s.kind = ApplicationResponse
				default:
					return s, fmt.Errorf("provider: unsupported root element %q", t.Name.Local)
				}
			}
			if depth == 2 && (t.Name.Local == "AccountingCustomerParty" || t.Name.Local == "DeliveryCustomerParty") {
				inCustomer = true
			}
			if inCustomer && s.receiver == "" && t.Name.Local == "ID" && len(stack) >= 2 && stack[len(stack)-2] == "PartyIdentification" {
				var v string
				scheme := ""
				for _, a := range t.Attr {
					if a.Name.Local == "schemeID" {
						scheme = a.Value
					}
				}
				if err := dec.DecodeElement(&v, &t); err != nil {
					return s, err
				}
				depth--
				stack = stack[:len(stack)-1]
				if scheme == "VKN" || scheme == "TCKN" {
					s.receiver = strings.TrimSpace(v)
				}
				continue
			}
			if depth == 2 {
				var dst *string
				switch t.Name.Local {
				case "UUID":
					dst = &s.uuid
				case "ID":
					dst = &s.id
				case "ProfileID":
					dst = &s.profile
				case "InvoiceTypeCode", "DespatchAdviceTypeCode":
					dst = &s.typ
				}
				if dst != nil {
					var v string
					if err := dec.DecodeElement(&v, &t); err != nil {
						return s, err
					}
					*dst = strings.TrimSpace(v)
					depth--
					stack = stack[:len(stack)-1]
				}
			}
		case xml.EndElement:
			if depth == 2 && (t.Name.Local == "AccountingCustomerParty" || t.Name.Local == "DeliveryCustomerParty") {
				inCustomer = false
			}
			depth--
			stack = stack[:len(stack)-1]
		}
	}
	if s.kind == EInvoice && s.profile == "EARSIVFATURA" {
		s.kind = EArchive
	}
	if s.kind == "" {
		return s, errors.New("provider: no root element")
	}
	return s, nil
}
