package ubltr_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/furkanmeclis/go-ubltr"
	"github.com/furkanmeclis/go-ubltr/schema/ubltr121/invoice"
)

func TestInspectInvoice(t *testing.T) {
	t.Parallel()

	sum, err := ubltr.Inspect(readFixture(t, "TemelFaturaOrnegi.xml"))
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if sum.Kind != ubltr.KindInvoice {
		t.Fatalf("Kind = %q", sum.Kind)
	}
	if sum.ID != "GIB20090000000001" {
		t.Fatalf("ID = %q", sum.ID)
	}
	if sum.Profile != "TEMELFATURA" {
		t.Fatalf("Profile = %q", sum.Profile)
	}
	if sum.Type != "SATIS" {
		t.Fatalf("Type = %q", sum.Type)
	}
	if sum.Currency != "TRY" {
		t.Fatalf("Currency = %q", sum.Currency)
	}
	if sum.Supplier != "AAA Anonim Şirketi" {
		t.Fatalf("Supplier = %q", sum.Supplier)
	}
	if sum.Lines != 1 {
		t.Fatalf("Lines = %d", sum.Lines)
	}
	if sum.Payable != "17.88" {
		t.Fatalf("Payable = %q", sum.Payable)
	}
	text := sum.String()
	for _, want := range []string{"Invoice", "GIB20090000000001", "TEMELFATURA", "AAA Anonim Şirketi", "17.88"} {
		if !strings.Contains(text, want) {
			t.Fatalf("String missing %q:\n%s", want, text)
		}
	}
}

func TestUnmarshalReaderAndMarshalWrite(t *testing.T) {
	t.Parallel()

	var inv invoice.Invoice
	if err := ubltr.UnmarshalReader(bytes.NewReader(readFixture(t, "TemelFaturaOrnegi.xml")), &inv); err != nil {
		t.Fatalf("UnmarshalReader: %v", err)
	}
	if inv.ID.Value != "GIB20090000000001" {
		t.Fatalf("ID = %q", inv.ID.Value)
	}
	var buf bytes.Buffer
	if err := ubltr.MarshalWrite(&buf, &inv); err != nil {
		t.Fatalf("MarshalWrite: %v", err)
	}
	if !strings.HasPrefix(buf.String(), `<?xml version="1.0"`) {
		t.Fatalf("missing xml declaration: %s", buf.String()[:min(80, buf.Len())])
	}
}
