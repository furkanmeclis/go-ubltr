package ubltr_test

import (
	"testing"

	"github.com/furkanmeclis/go-ubltr"
	"github.com/furkanmeclis/go-ubltr/schema/ubltr121/invoice"
)

func TestCloneInvoice(t *testing.T) {
	t.Parallel()

	var inv invoice.Invoice
	if err := ubltr.Unmarshal(readFixture(t, "TemelFaturaOrnegi.xml"), &inv); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	clone, err := ubltr.CloneInvoice(&inv)
	if err != nil {
		t.Fatalf("CloneInvoice: %v", err)
	}
	if clone == &inv {
		t.Fatal("clone shares pointer with source")
	}
	if len(clone.InvoiceLine) == 0 {
		t.Fatal("clone has no lines")
	}
	clone.InvoiceLine[0].ID.Value = "changed"
	if inv.InvoiceLine[0].ID.Value == "changed" {
		t.Fatal("clone shares InvoiceLine identity with source")
	}
	if clone.ID.Value != "GIB20090000000001" {
		t.Fatalf("clone ID = %q", clone.ID.Value)
	}
}

func TestCloneCreditNote(t *testing.T) {
	t.Parallel()

	var inv invoice.Invoice
	if err := ubltr.Unmarshal(readFixture(t, "TemelFaturaOrnegi.xml"), &inv); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	cn, err := ubltr.NewCreditNoteFromInvoice(&inv)
	if err != nil {
		t.Fatalf("NewCreditNoteFromInvoice: %v", err)
	}
	clone, err := ubltr.CloneCreditNote(cn)
	if err != nil {
		t.Fatalf("CloneCreditNote: %v", err)
	}
	if clone == cn {
		t.Fatal("clone shares pointer with source")
	}
	if len(clone.CreditNoteLine) == 0 {
		t.Fatal("clone has no lines")
	}
	clone.CreditNoteLine[0].ID.Value = "changed"
	if cn.CreditNoteLine[0].ID.Value == "changed" {
		t.Fatal("clone shares CreditNoteLine identity with source")
	}
}

func TestCloneInvoiceNil(t *testing.T) {
	t.Parallel()

	if _, err := ubltr.CloneInvoice(nil); err == nil {
		t.Fatal("expected error")
	}
}
