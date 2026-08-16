package builder_test

import (
	"errors"
	"testing"

	"github.com/furkanmeclis/go-ubltr"
	"github.com/furkanmeclis/go-ubltr/builder"
	"github.com/furkanmeclis/go-ubltr/codelist"
	"github.com/furkanmeclis/go-ubltr/schema/earchive"
	"github.com/furkanmeclis/go-ubltr/schema/ubltr121/creditnote"
)

func TestCreditNoteBuilderXSDValid(t *testing.T) {
	t.Parallel()

	cn, err := sampleCreditNote(t).Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if _, ok := any(cn).(*creditnote.CreditNote); !ok {
		t.Fatal("Build must return *creditnote.CreditNote")
	}

	data, err := ubltr.Marshal(cn)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if err := ubltr.ValidateXSD(t.Context(), data); err != nil {
		t.Fatalf("ValidateXSD: %v\n%s", err, data)
	}
	if cn.LegalMonetaryTotal.PayableAmount.Value != "24.00" {
		t.Fatalf("payable = %q", cn.LegalMonetaryTotal.PayableAmount.Value)
	}
}

func TestCreditNoteBuilderMissingFields(t *testing.T) {
	t.Parallel()

	_, err := builder.NewCreditNoteBuilder().Build()
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, builder.ErrBuilder) {
		t.Fatalf("error = %v", err)
	}
}

func TestApplicationResponseBuilderXSDValid(t *testing.T) {
	t.Parallel()

	doc, err := builder.NewApplicationResponseBuilder().
		Profile(codelist.ProfileTemelFatura).
		ID("APP-1").
		IssueDate("2026-03-02").
		SenderBuilder(companySupplier(t)).
		ReceiverBuilder(personCustomer(t)).
		DocumentResponse("GIB20260000000001", "2026-03-01", codelist.ResponseKabul).
		Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	data, err := ubltr.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if err := ubltr.ValidateXSD(t.Context(), data); err != nil {
		t.Fatalf("ValidateXSD: %v\n%s", err, data)
	}
}

func TestDespatchAdviceBuilderXSDValid(t *testing.T) {
	t.Parallel()

	qty, err := builder.QuantityIn(codelist.UnitC62, "2")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := builder.NewDespatchAdviceBuilder().
		Profile(codelist.ProfileTemelIrsaliye).
		Type(codelist.DespatchSEVK).
		ID("IRS20260000000001").
		IssueDate("2026-03-01").
		IssueTime("14:00:00").
		ShipmentID("SHIP-1").
		SupplierBuilder(companySupplier(t)).
		CustomerBuilder(personCustomer(t)).
		Line(builder.NewDespatchLineBuilder().
			Name("Widget").
			Quantity(qty)).
		Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	data, err := ubltr.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if err := ubltr.ValidateXSD(t.Context(), data); err != nil {
		t.Fatalf("ValidateXSD: %v\n%s", err, data)
	}
}

func TestReceiptAdviceBuilderXSDValid(t *testing.T) {
	t.Parallel()

	qty, err := builder.QuantityIn(codelist.UnitC62, "2")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := builder.NewReceiptAdviceBuilder().
		Profile(codelist.ProfileTemelIrsaliye).
		Type(codelist.ReceiptSEVK).
		ID("KBL20260000000001").
		IssueDate("2026-03-02").
		DespatchReference("IRS20260000000001", "2026-03-01").
		SupplierBuilder(companySupplier(t)).
		CustomerBuilder(personCustomer(t)).
		Line(builder.NewReceiptLineBuilder().
			Name("Widget").
			Quantity(qty)).
		Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	data, err := ubltr.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if err := ubltr.ValidateXSD(t.Context(), data); err != nil {
		t.Fatalf("ValidateXSD: %v\n%s", err, data)
	}
}

func sampleCreditNote(t *testing.T) *builder.CreditNoteBuilder {
	t.Helper()
	price, err := builder.MoneyIn(codelist.TRY, "10.00")
	if err != nil {
		t.Fatal(err)
	}
	qty, err := builder.QuantityIn(codelist.UnitC62, "2")
	if err != nil {
		t.Fatal(err)
	}
	return builder.NewCreditNoteBuilder().
		Profile(codelist.ProfileTemelFatura).
		Type(codelist.InvoiceTypeIade).
		ID("GIB20260000000002").
		IssueDate("2026-03-02").
		Currency(codelist.TRY).
		SupplierBuilder(companySupplier(t)).
		CustomerBuilder(personCustomer(t)).
		Line(builder.NewCreditNoteLineBuilder().
			Name("Widget").
			Quantity(qty).
			Price(price).
			Tax(builder.NewTaxBuilder().KDV("20")))
}

func TestEArchiveReportBuilder(t *testing.T) {
	t.Parallel()

	rep, err := builder.NewEArchiveReportBuilder().
		Version("1.0").
		MukellefVKN("1288331521").
		HazirlayanVKN("1288331521").
		Period("2026-03-01", "2026-03-31").
		Section("2026-03-01", "2026-03-31", "1").
		SignatureInnerXML("<SignedInfo></SignedInfo>").
		Invoice(earchive.Fatura{
			FaturaNo:         "GIB20260000000001",
			FaturaUUID:       "AAAAAAAA-BBBB-CCCC-DDDD-EEEEEEEEEEEE",
			FaturaTip:        earchive.FaturaTipYTBTEVKIFAT,
			GonderimSekli:    earchive.GonderimSekliELEKTRONIK,
			DosyaAdi:         "fatura.xml",
			OzetDeger:        "abc",
			DuzenlenmeTarihi: "2026-03-01",
			DuzenlenmeZamani: "10:00:00",
			ToplamTutar:      "10.00",
			ToplamIskonto:    "0.00",
			OdenecekTutar:    "12.00",
			ParaBirimi:       "TRY",
			DovizKuru:        "1",
			FaturaUrl:        "https://example.invalid/fatura.xml",
			ImzaZamani:       "2026-03-01T10:00:00",
		}).
		Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if _, ok := any(rep).(*earchive.EArsivRaporu); !ok {
		t.Fatal("Build must return *earchive.EArsivRaporu")
	}
	data, err := ubltr.Marshal(rep)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	kind, err := ubltr.Detect(data)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if kind != ubltr.KindEArchiveReport {
		t.Fatalf("kind = %q", kind)
	}
}

func TestEArchiveReportBuilderMissingFields(t *testing.T) {
	t.Parallel()

	_, err := builder.NewEArchiveReportBuilder().Build()
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, builder.ErrBuilder) {
		t.Fatalf("error = %v", err)
	}
}
