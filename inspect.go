package ubltr

import (
	"fmt"
	"io"
	"strings"

	"github.com/furkanmeclis/go-ubltr/schema/ubltr121/cac"
	"github.com/furkanmeclis/go-ubltr/schema/ubltr121/creditnote"
	"github.com/furkanmeclis/go-ubltr/schema/ubltr121/invoice"
)

// Summary is a human-readable view of a document. It is not a second document model.
type Summary struct {
	Kind     Kind
	ID       string
	UUID     string
	Profile  string
	Type     string
	Currency string
	Supplier string
	Customer string
	Lines    int
	Tax      string
	Payable  string
}

func (s Summary) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n", summaryTitle(s.Kind))
	fmt.Fprintf(&b, "ID: %s\n", s.ID)
	fmt.Fprintf(&b, "UUID: %s\n", s.UUID)
	fmt.Fprintf(&b, "Profile: %s\n", s.Profile)
	fmt.Fprintf(&b, "Type: %s\n", s.Type)
	fmt.Fprintf(&b, "Currency: %s\n", s.Currency)
	fmt.Fprintf(&b, "Supplier: %s\n", s.Supplier)
	fmt.Fprintf(&b, "Customer: %s\n", s.Customer)
	fmt.Fprintf(&b, "Lines: %d\n", s.Lines)
	fmt.Fprintf(&b, "Tax: %s\n", s.Tax)
	fmt.Fprintf(&b, "Payable: %s", s.Payable)
	return b.String()
}

func summaryTitle(k Kind) string {
	switch k {
	case KindInvoice:
		return "Invoice"
	case KindCreditNote:
		return "CreditNote"
	case KindApplicationResponse:
		return "ApplicationResponse"
	case KindDespatchAdvice:
		return "DespatchAdvice"
	case KindReceiptAdvice:
		return "ReceiptAdvice"
	case KindEArchiveReport:
		return "eArsivRaporu"
	default:
		return "Document"
	}
}

// Inspect reports ID, profile, parties, line count, and payable from xmlBytes.
// It uses Detect and unmarshal. It does not run XSD or Schematron.
func Inspect(xmlBytes []byte) (Summary, error) {
	kind, err := Detect(xmlBytes)
	if err != nil {
		return Summary{}, err
	}
	switch kind {
	case KindInvoice:
		var inv invoice.Invoice
		if err := Unmarshal(xmlBytes, &inv); err != nil {
			return Summary{}, err
		}
		return inspectInvoice(&inv), nil
	case KindCreditNote:
		var cn creditnote.CreditNote
		if err := Unmarshal(xmlBytes, &cn); err != nil {
			return Summary{}, err
		}
		return inspectCreditNote(&cn), nil
	default:
		return Summary{Kind: kind}, nil
	}
}

func inspectInvoice(inv *invoice.Invoice) Summary {
	tax := ""
	if len(inv.TaxTotal) > 0 {
		tax = inv.TaxTotal[0].TaxAmount.Value
	}
	return Summary{
		Kind:     KindInvoice,
		ID:       inv.ID.Value,
		UUID:     inv.UUID.Value,
		Profile:  inv.ProfileID.Value,
		Type:     inv.InvoiceTypeCode.Value,
		Currency: inv.DocumentCurrencyCode.Value,
		Supplier: partyName(inv.AccountingSupplierParty.Party),
		Customer: partyName(inv.AccountingCustomerParty.Party),
		Lines:    len(inv.InvoiceLine),
		Tax:      tax,
		Payable:  inv.LegalMonetaryTotal.PayableAmount.Value,
	}
}

func inspectCreditNote(cn *creditnote.CreditNote) Summary {
	tax := ""
	if len(cn.TaxTotal) > 0 {
		tax = cn.TaxTotal[0].TaxAmount.Value
	}
	profile, typ, currency := "", "", ""
	if cn.ProfileID != nil {
		profile = cn.ProfileID.Value
	}
	if cn.CreditNoteTypeCode != nil {
		typ = cn.CreditNoteTypeCode.Value
	}
	if cn.DocumentCurrencyCode != nil {
		currency = cn.DocumentCurrencyCode.Value
	}
	uuid := ""
	if cn.UUID != nil {
		uuid = cn.UUID.Value
	}
	return Summary{
		Kind:     KindCreditNote,
		ID:       cn.ID.Value,
		UUID:     uuid,
		Profile:  profile,
		Type:     typ,
		Currency: currency,
		Supplier: partyName(cn.AccountingSupplierParty.Party),
		Customer: partyName(cn.AccountingCustomerParty.Party),
		Lines:    len(cn.CreditNoteLine),
		Tax:      tax,
		Payable:  cn.LegalMonetaryTotal.PayableAmount.Value,
	}
}

func partyName(p cac.PartyType) string {
	if p.PartyName != nil && p.PartyName.Name.Value != "" {
		return p.PartyName.Name.Value
	}
	if p.Person != nil {
		name := strings.TrimSpace(p.Person.FirstName.Value + " " + p.Person.FamilyName.Value)
		if name != "" {
			return name
		}
	}
	for _, id := range p.PartyIdentification {
		if id.ID.Value != "" {
			return id.ID.Value
		}
	}
	return ""
}

// UnmarshalReader reads all of r and unmarshals it into v.
func UnmarshalReader(r io.Reader, v any) error {
	if r == nil {
		return fmt.Errorf("nil reader")
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return fmt.Errorf("read xml: %w", err)
	}
	return Unmarshal(data, v)
}

// MarshalWrite marshals v and writes it to w, including the XML declaration.
func MarshalWrite(w io.Writer, v any) error {
	if w == nil {
		return fmt.Errorf("nil writer")
	}
	data, err := Marshal(v)
	if err != nil {
		return err
	}
	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("write xml: %w", err)
	}
	return nil
}
