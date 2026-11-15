package builder_test

import (
	"encoding/base64"
	"errors"
	"testing"

	"github.com/furkanmeclis/go-ubltr"
	"github.com/furkanmeclis/go-ubltr/builder"
	"github.com/furkanmeclis/go-ubltr/codelist"
	"github.com/furkanmeclis/go-ubltr/schema/ubltr121/invoice"
)

func TestWithholdingTotals(t *testing.T) {
	t.Parallel()

	inv, err := goldenTevkifat(t)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	tests := []struct {
		name, got, want string
	}{
		{"line 1 withheld", inv.InvoiceLine[0].WithholdingTaxTotal[0].TaxAmount.Value, "1400.00"},
		{"line 1 withholding base", inv.InvoiceLine[0].WithholdingTaxTotal[0].TaxSubtotal[0].TaxableAmount.Value, "2000.00"},
		{"line 2 withheld", inv.InvoiceLine[1].WithholdingTaxTotal[0].TaxAmount.Value, "200.00"},
		{"document withheld", inv.WithholdingTaxTotal[0].TaxAmount.Value, "1600.00"},
		{"document KDV", inv.TaxTotal[0].TaxAmount.Value, "3000.00"},
		{"tax inclusive", inv.LegalMonetaryTotal.TaxInclusiveAmount.Value, "18000.00"},
		{"payable", inv.LegalMonetaryTotal.PayableAmount.Value, "16400.00"},
	}
	for _, tc := range tests {
		if tc.got != tc.want {
			t.Errorf("%s = %q, want %q", tc.name, tc.got, tc.want)
		}
	}
	if len(inv.WithholdingTaxTotal[0].TaxSubtotal) != 2 {
		t.Fatalf("document withholding subtotals = %d, want 2", len(inv.WithholdingTaxTotal[0].TaxSubtotal))
	}
}

func TestWithholdingPercentCanonical(t *testing.T) {
	t.Parallel()

	inv, err := goldenBase(codelist.ProfileTemelFatura, codelist.InvoiceTypeTevkifat, "GIB2026000000003", "").
		SupplierBuilder(goldenSupplier()).
		CustomerBuilder(goldenCompanyCustomer()).
		Line(goldenLine(t, codelist.TRY, "Hizmet", "1", "100.00",
			builder.NewTaxBuilder().KDV("20").Withholding(codelist.Withholding603, "70.00"))).
		Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if got := inv.InvoiceLine[0].WithholdingTaxTotal[0].TaxSubtotal[0].Percent.Value; got != "70" {
		t.Fatalf("percent = %q, want 70", got)
	}
}

func TestInvoiceRuleErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		build func(t *testing.T) (*invoice.Invoice, error)
		field string
	}{
		{
			name: "withholding percent not in list",
			build: func(t *testing.T) (*invoice.Invoice, error) {
				return goldenBase(codelist.ProfileTemelFatura, codelist.InvoiceTypeTevkifat, "GIB2026000000003", "").
					SupplierBuilder(goldenSupplier()).
					CustomerBuilder(goldenCompanyCustomer()).
					Line(goldenLine(t, codelist.TRY, "Hizmet", "1", "100.00",
						builder.NewTaxBuilder().KDV("20").Withholding(codelist.Withholding603, "55"))).
					Build()
			},
			field: "tax.withholding.percent",
		},
		{
			name: "withholding on SATIS",
			build: func(t *testing.T) (*invoice.Invoice, error) {
				return goldenBase(codelist.ProfileTemelFatura, codelist.InvoiceTypeSatis, "GIB2026000000003", "").
					SupplierBuilder(goldenSupplier()).
					CustomerBuilder(goldenCompanyCustomer()).
					Line(goldenLine(t, codelist.TRY, "Hizmet", "1", "100.00",
						builder.NewTaxBuilder().KDV("20").Withholding(codelist.Withholding603, "70"))).
					Build()
			},
			field: "withholding",
		},
		{
			name: "TEVKIFAT without withholding",
			build: func(t *testing.T) (*invoice.Invoice, error) {
				return goldenBase(codelist.ProfileTemelFatura, codelist.InvoiceTypeTevkifat, "GIB2026000000003", "").
					SupplierBuilder(goldenSupplier()).
					CustomerBuilder(goldenCompanyCustomer()).
					Line(goldenLine(t, codelist.TRY, "Hizmet", "1", "100.00", builder.NewTaxBuilder().KDV("20"))).
					Build()
			},
			field: "withholding",
		},
		{
			name: "IADE without billing reference",
			build: func(t *testing.T) (*invoice.Invoice, error) {
				return goldenBase(codelist.ProfileTemelFatura, codelist.InvoiceTypeIade, "GIB2026000000005", "").
					SupplierBuilder(goldenSupplier()).
					CustomerBuilder(goldenCompanyCustomer()).
					Line(goldenLine(t, codelist.TRY, "Hizmet", "1", "100.00", builder.NewTaxBuilder().KDV("20"))).
					Build()
			},
			field: "billingReference",
		},
		{
			name: "USD without exchange rate",
			build: func(t *testing.T) (*invoice.Invoice, error) {
				return goldenBase(codelist.ProfileTicariFatura, codelist.InvoiceTypeSatis, "GIB2026000000007", "").
					Currency(codelist.USD).
					SupplierBuilder(goldenSupplier()).
					CustomerBuilder(goldenCompanyCustomer()).
					Line(goldenLine(t, codelist.USD, "Lisans", "1", "10.00", builder.NewTaxBuilder().KDV("20"))).
					Build()
			},
			field: "exchangeRate",
		},
		{
			name: "exchange rate with too many digits",
			build: func(t *testing.T) (*invoice.Invoice, error) {
				return goldenBase(codelist.ProfileTicariFatura, codelist.InvoiceTypeSatis, "GIB2026000000007", "").
					Currency(codelist.USD).
					ExchangeRate(codelist.USD, codelist.TRY, "34.1234567", "").
					SupplierBuilder(goldenSupplier()).
					CustomerBuilder(goldenCompanyCustomer()).
					Line(goldenLine(t, codelist.USD, "Lisans", "1", "10.00", builder.NewTaxBuilder().KDV("20"))).
					Build()
			},
			field: "exchangeRate.rate",
		},
		{
			name: "IHRACAT without buyer",
			build: func(t *testing.T) (*invoice.Invoice, error) {
				return goldenBase(codelist.ProfileIhracat, codelist.InvoiceTypeIstisna, "GIB2026000000006", "").
					Currency(codelist.EUR).
					ExchangeRate(codelist.EUR, codelist.TRY, "37.5", "").
					SupplierBuilder(goldenSupplier()).
					CustomerBuilder(builder.NewExportCustomerBuilder()).
					Line(goldenLine(t, codelist.EUR, "Tişört", "1", "4.50",
						builder.NewTaxBuilder().KDV("0").Exemption(codelist.Exemption301, "11/1-a Mal ihracatı"))).
					Build()
			},
			field: "buyerCustomer",
		},
		{
			name: "unknown exemption code",
			build: func(t *testing.T) (*invoice.Invoice, error) {
				return goldenBase(codelist.ProfileTemelFatura, codelist.InvoiceTypeIstisna, "GIB2026000000004", "").
					SupplierBuilder(goldenSupplier()).
					CustomerBuilder(goldenCompanyCustomer()).
					Line(goldenLine(t, codelist.TRY, "Teslim", "1", "100.00",
						builder.NewTaxBuilder().KDV("0").Exemption("999", "?"))).
					Build()
			},
			field: "tax.exemption",
		},
		{
			name: "unknown incoterm",
			build: func(t *testing.T) (*invoice.Invoice, error) {
				return goldenBase(codelist.ProfileTemelFatura, codelist.InvoiceTypeSatis, "GIB2026000000001", "").
					SupplierBuilder(goldenSupplier()).
					CustomerBuilder(goldenCompanyCustomer()).
					Line(goldenLine(t, codelist.TRY, "Teslim", "1", "100.00", builder.NewTaxBuilder().KDV("20")).
						Delivery(builder.NewDeliveryBuilder().Incoterms("XYZ"))).
					Build()
			},
			field: "delivery.incoterms",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := tc.build(t)
			if !errors.Is(err, builder.ErrBuilder) {
				t.Fatalf("error = %v, want ErrBuilder", err)
			}
			var be *builder.BuilderError
			if !errors.As(err, &be) || be.Field != tc.field {
				t.Fatalf("error = %v, want field %s", err, tc.field)
			}
		})
	}
}

func TestReturnInvoiceFrom(t *testing.T) {
	t.Parallel()

	orig, err := goldenTicariSatis(t)
	if err != nil {
		t.Fatal(err)
	}
	ret, err := goldenIade(t)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if ret.InvoiceTypeCode.Value != codelist.InvoiceTypeIade {
		t.Fatalf("type = %q", ret.InvoiceTypeCode.Value)
	}
	if ret.ProfileID.Value != codelist.ProfileTemelFatura {
		t.Fatalf("profile = %q, want TEMELFATURA for a TICARIFATURA return", ret.ProfileID.Value)
	}
	if ret.UUID.Value == orig.UUID.Value {
		t.Fatal("return must have its own UUID")
	}
	if got := ret.AccountingSupplierParty.Party.PartyIdentification[0].ID.Value; got != "9205121120" {
		t.Fatalf("supplier = %q, want original customer", got)
	}
	if got := ret.AccountingCustomerParty.Party.PartyIdentification[0].ID.Value; got != "1288331521" {
		t.Fatalf("customer = %q, want original supplier", got)
	}
	if len(ret.BillingReference) != 1 {
		t.Fatalf("billing references = %d", len(ret.BillingReference))
	}
	ref := ret.BillingReference[0].InvoiceDocumentReference
	if ref.ID.Value != orig.ID.Value || ref.IssueDate.Value != orig.IssueDate.Value {
		t.Fatalf("reference = %s %s", ref.ID.Value, ref.IssueDate.Value)
	}
	if ref.DocumentTypeCode == nil || ref.DocumentTypeCode.Value != builder.ReturnDocumentTypeCode {
		t.Fatalf("document type code = %v", ref.DocumentTypeCode)
	}
	if ret.LegalMonetaryTotal.PayableAmount.Value != orig.LegalMonetaryTotal.PayableAmount.Value {
		t.Fatalf("payable = %q, want %q", ret.LegalMonetaryTotal.PayableAmount.Value, orig.LegalMonetaryTotal.PayableAmount.Value)
	}
}

func TestReturnInvoiceFromTevkifat(t *testing.T) {
	t.Parallel()

	orig, err := goldenTevkifat(t)
	if err != nil {
		t.Fatal(err)
	}
	ret, err := builder.NewReturnInvoiceFrom(orig, builder.ReturnLines("2")).
		ID("GIB2026000000008").
		IssueDate("2026-03-12").
		Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if ret.InvoiceTypeCode.Value != codelist.InvoiceTypeIade {
		t.Fatalf("type = %q, want IADE", ret.InvoiceTypeCode.Value)
	}
	if len(ret.WithholdingTaxTotal) != 1 || ret.WithholdingTaxTotal[0].TaxAmount.Value != "200.00" {
		t.Fatalf("withholding = %+v", ret.WithholdingTaxTotal)
	}
	if len(ret.InvoiceLine) != 1 || ret.InvoiceLine[0].ID.Value != "2" {
		t.Fatalf("lines = %+v", ret.InvoiceLine)
	}
	if ret.LegalMonetaryTotal.PayableAmount.Value != "5800.00" {
		t.Fatalf("payable = %q, want 5800.00", ret.LegalMonetaryTotal.PayableAmount.Value)
	}
	data, err := ubltr.Marshal(ret)
	if err != nil {
		t.Fatal(err)
	}
	if err := ubltr.ValidateXSD(t.Context(), data); err != nil {
		t.Fatalf("ValidateXSD: %v", err)
	}
	if err := ubltr.ValidateSchematron(t.Context(), data); err != nil {
		t.Fatalf("ValidateSchematron: %v", err)
	}
}

func TestXSLTReference(t *testing.T) {
	t.Parallel()

	inv, err := goldenTicariSatis(t)
	if err != nil {
		t.Fatal(err)
	}
	if len(inv.AdditionalDocumentReference) != 1 {
		t.Fatalf("additional references = %d", len(inv.AdditionalDocumentReference))
	}
	ref := inv.AdditionalDocumentReference[0]
	if ref.ID.Value != inv.UUID.Value {
		t.Fatalf("id = %q, want invoice UUID", ref.ID.Value)
	}
	if ref.DocumentType == nil || ref.DocumentType.Value != builder.XSLTDocumentType {
		t.Fatalf("document type = %v", ref.DocumentType)
	}
	bin := ref.Attachment.EmbeddedDocumentBinaryObject
	if bin.MimeCode != "application/xml" {
		t.Fatalf("mime = %q", bin.MimeCode)
	}
	raw, err := base64.StdEncoding.DecodeString(bin.Value)
	if err != nil || string(raw) != goldenXSLT {
		t.Fatalf("decoded stylesheet mismatch: %v", err)
	}
	if ref.DocumentDescription[0].Value != "fatura.xslt" {
		t.Fatalf("description = %q", ref.DocumentDescription[0].Value)
	}

	if _, err := builder.XSLTReference("", "2026-03-01", "a.xslt", []byte("x")); !errors.Is(err, builder.ErrBuilder) {
		t.Fatalf("empty id error = %v", err)
	}
}

func TestReferencesAndExchangeRate(t *testing.T) {
	t.Parallel()

	inv, err := goldenTicariSatis(t)
	if err != nil {
		t.Fatal(err)
	}
	if inv.OrderReference == nil || inv.OrderReference.ID.Value != "SIP-2026-0042" {
		t.Fatalf("order reference = %+v", inv.OrderReference)
	}
	if len(inv.DespatchDocumentReference) != 1 || inv.DespatchDocumentReference[0].IssueDate.Value != "2026-02-28" {
		t.Fatalf("despatch reference = %+v", inv.DespatchDocumentReference)
	}
	if got := inv.Note[len(inv.Note)-1].Value; got != "YALNIZ BEŞ BİN DÖRT YÜZ TÜRK LİRASI" {
		t.Fatalf("amount in words note = %q", got)
	}

	usd, err := goldenTicariUSD(t)
	if err != nil {
		t.Fatal(err)
	}
	er := usd.PricingExchangeRate
	if er == nil || er.SourceCurrencyCode.Value != "USD" || er.TargetCurrencyCode.Value != "TRY" || er.CalculationRate.Value != "34.5678" {
		t.Fatalf("exchange rate = %+v", er)
	}
}

// TestSchematronRejectsBrokenExport checks that the golden IHRACAT rules
// are really exercised: dropping the buyer must fail Schematron.
func TestSchematronRejectsBrokenExport(t *testing.T) {
	t.Parallel()

	inv, err := goldenIhracat(t)
	if err != nil {
		t.Fatal(err)
	}
	inv.BuyerCustomerParty = nil
	inv.PricingExchangeRate = nil
	data, err := ubltr.Marshal(inv)
	if err != nil {
		t.Fatal(err)
	}
	err = ubltr.ValidateSchematron(t.Context(), data)
	var se *ubltr.SchematronValidationError
	if !errors.As(err, &se) {
		t.Fatalf("error = %v, want SchematronValidationError", err)
	}
	if len(se.Violations) < 2 {
		t.Fatalf("violations = %v, want buyer and exchange-rate failures", se.Messages)
	}
}

func TestAmountInWords(t *testing.T) {
	t.Parallel()

	tests := []struct {
		currency, amount, want string
	}{
		{"TRY", "123.45", "YALNIZ YÜZ YİRMİ ÜÇ TÜRK LİRASI KIRK BEŞ KURUŞ"},
		{"TRY", "0", "YALNIZ SIFIR TÜRK LİRASI"},
		{"TRY", "0.05", "YALNIZ SIFIR TÜRK LİRASI BEŞ KURUŞ"},
		{"TRY", "1", "YALNIZ BİR TÜRK LİRASI"},
		{"TRY", "10", "YALNIZ ON TÜRK LİRASI"},
		{"TRY", "100", "YALNIZ YÜZ TÜRK LİRASI"},
		{"TRY", "1000", "YALNIZ BİN TÜRK LİRASI"},
		{"TRY", "1001", "YALNIZ BİN BİR TÜRK LİRASI"},
		{"TRY", "2000", "YALNIZ İKİ BİN TÜRK LİRASI"},
		{"TRY", "11000", "YALNIZ ON BİR BİN TÜRK LİRASI"},
		{"TRY", "101000", "YALNIZ YÜZ BİR BİN TÜRK LİRASI"},
		{"TRY", "999999.99", "YALNIZ DOKUZ YÜZ DOKSAN DOKUZ BİN DOKUZ YÜZ DOKSAN DOKUZ TÜRK LİRASI DOKSAN DOKUZ KURUŞ"},
		{"TRY", "1000000", "YALNIZ BİR MİLYON TÜRK LİRASI"},
		{"TRY", "2500000.10", "YALNIZ İKİ MİLYON BEŞ YÜZ BİN TÜRK LİRASI ON KURUŞ"},
		{"TRY", "1000000000", "YALNIZ BİR MİLYAR TÜRK LİRASI"},
		{"TRY", "3001002003", "YALNIZ ÜÇ MİLYAR BİR MİLYON İKİ BİN ÜÇ TÜRK LİRASI"},
		{"TRY", "12.345", "YALNIZ ON İKİ TÜRK LİRASI OTUZ BEŞ KURUŞ"},
		{"TRY", "-5", "YALNIZ EKSİ BEŞ TÜRK LİRASI"},
		{"USD", "1250.50", "YALNIZ BİN İKİ YÜZ ELLİ ABD DOLARI ELLİ SENT"},
		{"EUR", "60.06", "YALNIZ ALTMIŞ AVRO ALTI SENT"},
		{"JPY", "1500", "YALNIZ BİN BEŞ YÜZ JAPON YENİ"},
		{"XAU", "7", "YALNIZ YEDİ XAU"},
	}
	for _, tc := range tests {
		t.Run(tc.currency+" "+tc.amount, func(t *testing.T) {
			t.Parallel()
			got := builder.AmountInWords(mustMoney(t, tc.currency, tc.amount))
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestAmountInWordsNoteNeedsTotals(t *testing.T) {
	t.Parallel()

	_, err := sampleInvoice(t).ManualTotals().AmountInWordsNote().Build()
	var be *builder.BuilderError
	if !errors.As(err, &be) || be.Field != "amountInWords" {
		t.Fatalf("error = %v, want amountInWords", err)
	}
}

func TestCreditNoteRejectsWithholding(t *testing.T) {
	t.Parallel()

	_, err := builder.NewCreditNoteLineBuilder().
		Name("Hizmet").
		Quantity(mustQty(t, codelist.UnitC62, "1")).
		Price(mustMoney(t, codelist.TRY, "100")).
		Tax(builder.NewTaxBuilder().KDV("20").Withholding(codelist.Withholding603, "70")).
		Build()
	if !errors.Is(err, builder.ErrBuilder) {
		t.Fatalf("error = %v, want ErrBuilder", err)
	}
}
