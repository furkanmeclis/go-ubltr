package builder_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/furkanmeclis/go-ubltr"
	"github.com/furkanmeclis/go-ubltr/builder"
	"github.com/furkanmeclis/go-ubltr/codelist"
	"github.com/furkanmeclis/go-ubltr/schema/ubltr121/invoice"
)

func TestInvoiceBuilderXSDValid(t *testing.T) {
	t.Parallel()

	inv, err := sampleInvoice(t).Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if _, ok := any(inv).(*invoice.Invoice); !ok {
		t.Fatal("Build must return *invoice.Invoice")
	}

	data, err := ubltr.Marshal(inv)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if err := ubltr.ValidateXSD(t.Context(), data); err != nil {
		t.Fatalf("ValidateXSD: %v\n%s", err, data)
	}

	if inv.ID.Value != "GIB2026000000001" {
		t.Fatalf("ID = %q", inv.ID.Value)
	}
	if inv.LegalMonetaryTotal.PayableAmount.Value != "24.00" {
		t.Fatalf("payable = %q, want 24.00", inv.LegalMonetaryTotal.PayableAmount.Value)
	}
	if inv.LegalMonetaryTotal.LineExtensionAmount.Value != "20.00" {
		t.Fatalf("line extension = %q", inv.LegalMonetaryTotal.LineExtensionAmount.Value)
	}
	if inv.TaxTotal[0].TaxAmount.Value != "4.00" {
		t.Fatalf("tax = %q", inv.TaxTotal[0].TaxAmount.Value)
	}
	if inv.LineCountNumeric.Value != "1" {
		t.Fatalf("line count = %q", inv.LineCountNumeric.Value)
	}
	if inv.UUID.Value == "" {
		t.Fatal("expected generated UUID")
	}
}

func TestInvoiceBuilderRoundingFixture(t *testing.T) {
	t.Parallel()

	price, err := builder.MoneyIn(codelist.TRY, "0.15")
	if err != nil {
		t.Fatal(err)
	}
	qty, err := builder.QuantityIn(codelist.UnitKWH, "101")
	if err != nil {
		t.Fatal(err)
	}

	inv, err := baseInvoice(t).
		Line(builder.NewInvoiceLineBuilder().
			Name("Elektrik Tüketim Bedeli").
			Quantity(qty).
			Price(price).
			Tax(builder.NewTaxBuilder().KDV("18"))).
		Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	tests := []struct {
		name, got, want string
	}{
		{"line extension", inv.InvoiceLine[0].LineExtensionAmount.Value, "15.15"},
		{"tax exclusive", inv.LegalMonetaryTotal.TaxExclusiveAmount.Value, "15.15"},
		{"tax", inv.TaxTotal[0].TaxAmount.Value, "2.73"},
		{"payable", inv.LegalMonetaryTotal.PayableAmount.Value, "17.88"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if tc.got != tc.want {
				t.Fatalf("got %q, want %q", tc.got, tc.want)
			}
		})
	}
}

func TestInvoiceBuilderRejectsMissingFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		build func() (*invoice.Invoice, error)
		field string
	}{
		{
			name: "no supplier",
			build: func() (*invoice.Invoice, error) {
				return builder.NewInvoiceBuilder().
					Profile(codelist.ProfileTemelFatura).
					Type(codelist.InvoiceTypeSatis).
					ID("GIB2026000000001").
					IssueDate("2026-03-01").
					CustomerBuilder(personCustomer(t)).
					Line(oneLine(t)).
					Build()
			},
			field: "supplier",
		},
		{
			name: "no lines",
			build: func() (*invoice.Invoice, error) {
				return baseInvoice(t).Build()
			},
			field: "line",
		},
		{
			name: "no id",
			build: func() (*invoice.Invoice, error) {
				return builder.NewInvoiceBuilder().
					Profile(codelist.ProfileTemelFatura).
					Type(codelist.InvoiceTypeSatis).
					IssueDate("2026-03-01").
					SupplierBuilder(companySupplier(t)).
					CustomerBuilder(personCustomer(t)).
					Line(oneLine(t)).
					Build()
			},
			field: "id",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := tc.build()
			if err == nil {
				t.Fatal("expected builder error")
			}
			if !errors.Is(err, builder.ErrBuilder) {
				t.Fatalf("error = %v, want ErrBuilder", err)
			}
			var be *builder.BuilderError
			if !errors.As(err, &be) || be.Field != tc.field {
				t.Fatalf("field = %v, want %s", err, tc.field)
			}
		})
	}
}

func TestInvoiceBuilderCurrencyMismatch(t *testing.T) {
	t.Parallel()

	usd, err := builder.MoneyIn(codelist.USD, "10.00")
	if err != nil {
		t.Fatal(err)
	}
	qty, err := builder.QuantityIn(codelist.UnitC62, "1")
	if err != nil {
		t.Fatal(err)
	}

	_, err = baseInvoice(t).
		Line(builder.NewInvoiceLineBuilder().
			Name("Widget").
			Quantity(qty).
			Price(usd).
			Tax(builder.NewTaxBuilder().KDV("20"))).
		Build()
	if err == nil {
		t.Fatal("expected currency error")
	}
	if !errors.Is(err, builder.ErrBuilder) {
		t.Fatalf("error = %v", err)
	}
}

func TestInvoiceBuilderDocumentAllowance(t *testing.T) {
	t.Parallel()

	disc, err := builder.MoneyIn(codelist.TRY, "2.00")
	if err != nil {
		t.Fatal(err)
	}
	inv, err := sampleInvoice(t).Allowance(disc, "iskonto").Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if inv.LegalMonetaryTotal.TaxExclusiveAmount.Value != "18.00" {
		t.Fatalf("tax exclusive = %q, want 18.00", inv.LegalMonetaryTotal.TaxExclusiveAmount.Value)
	}
	if inv.LegalMonetaryTotal.AllowanceTotalAmount == nil || inv.LegalMonetaryTotal.AllowanceTotalAmount.Value != "2.00" {
		t.Fatalf("allowance total = %v", inv.LegalMonetaryTotal.AllowanceTotalAmount)
	}
}

func TestInvoiceBuilderPaymentAndDelivery(t *testing.T) {
	t.Parallel()

	addr, err := turkeyAddress().Build()
	if err != nil {
		t.Fatal(err)
	}
	inv, err := sampleInvoice(t).
		Payment(builder.NewPaymentBuilder().
			Code(codelist.PaymentMeansCreditTransfer).
			DueDate("2026-03-20").
			IBAN("TR330006100519786457841326").
			Note("havale")).
		Delivery(builder.NewDeliveryBuilder().
			Date("2026-03-02").
			Address(addr)).
		Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(inv.PaymentMeans) != 1 || inv.PaymentMeans[0].PaymentMeansCode.Value != codelist.PaymentMeansCreditTransfer {
		t.Fatalf("payment means = %+v", inv.PaymentMeans)
	}
	if inv.PaymentTerms == nil || inv.PaymentTerms.PaymentDueDate == nil {
		t.Fatal("expected payment terms")
	}
	if len(inv.Delivery) != 1 || inv.Delivery[0].DeliveryAddress == nil {
		t.Fatal("expected delivery address")
	}

	data, err := ubltr.Marshal(inv)
	if err != nil {
		t.Fatal(err)
	}
	if err := ubltr.ValidateXSD(t.Context(), data); err != nil {
		t.Fatalf("ValidateXSD: %v", err)
	}
}

func sampleInvoice(t *testing.T) *builder.InvoiceBuilder {
	t.Helper()
	return baseInvoice(t).Line(oneLine(t))
}

func baseInvoice(t *testing.T) *builder.InvoiceBuilder {
	t.Helper()
	return builder.NewInvoiceBuilder().
		Profile(codelist.ProfileTemelFatura).
		Type(codelist.InvoiceTypeSatis).
		ID("GIB2026000000001").
		IssueDate("2026-03-01").
		IssueTime("10:00:00").
		Currency(codelist.TRY).
		SupplierBuilder(companySupplier(t)).
		CustomerBuilder(personCustomer(t))
}

func oneLine(t *testing.T) *builder.InvoiceLineBuilder {
	t.Helper()
	price, err := builder.MoneyIn(codelist.TRY, "10.00")
	if err != nil {
		t.Fatal(err)
	}
	qty, err := builder.QuantityIn(codelist.UnitC62, "2")
	if err != nil {
		t.Fatal(err)
	}
	return builder.NewInvoiceLineBuilder().
		Name("Widget").
		Quantity(qty).
		Price(price).
		Tax(builder.NewTaxBuilder().KDV("20"))
}

func companySupplier(t *testing.T) *builder.PartyBuilder {
	t.Helper()
	return builder.NewPartyBuilder().
		VKN("1288331521").
		Name("AAA Anonim Şirketi").
		TaxOffice("Büyük Mükellefler").
		AddressBuilder(turkeyAddress())
}

func personCustomer(t *testing.T) *builder.PartyBuilder {
	t.Helper()
	return builder.NewPartyBuilder().
		TCKN("12345678901").
		Person("Ali", "Yılmaz").
		AddressBuilder(turkeyAddress())
}

func turkeyAddress() *builder.AddressBuilder {
	return builder.NewAddressBuilder().
		Street("Papatya Caddesi").
		Building("21").
		District("Beşiktaş").
		City("İstanbul").
		PostalZone("34100").
		Country("Türkiye").
		CountryCode("TR")
}

func TestBuilderErrorString(t *testing.T) {
	t.Parallel()

	_, err := builder.NewInvoiceBuilder().Build()
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "builder:") {
		t.Fatalf("error = %q", err)
	}
}
