package ubltr

import (
	"fmt"

	"github.com/furkanmeclis/go-ubltr/schema/ubltr121/creditnote"
	"github.com/furkanmeclis/go-ubltr/schema/ubltr121/invoice"
)

// CloneInvoice returns a deep copy of inv. The copy does not share slice or
// pointer identity with the source.
func CloneInvoice(inv *invoice.Invoice) (*invoice.Invoice, error) {
	if inv == nil {
		return nil, fmt.Errorf("nil invoice")
	}
	data, err := Marshal(inv)
	if err != nil {
		return nil, fmt.Errorf("clone invoice: %w", err)
	}
	out := NewInvoice()
	if err := Unmarshal(data, out); err != nil {
		return nil, fmt.Errorf("clone invoice: %w", err)
	}
	return out, nil
}

// CloneCreditNote returns a deep copy of cn. The copy does not share slice or
// pointer identity with the source.
func CloneCreditNote(cn *creditnote.CreditNote) (*creditnote.CreditNote, error) {
	if cn == nil {
		return nil, fmt.Errorf("nil credit note")
	}
	data, err := Marshal(cn)
	if err != nil {
		return nil, fmt.Errorf("clone credit note: %w", err)
	}
	out := NewCreditNote()
	if err := Unmarshal(data, out); err != nil {
		return nil, fmt.Errorf("clone credit note: %w", err)
	}
	return out, nil
}
