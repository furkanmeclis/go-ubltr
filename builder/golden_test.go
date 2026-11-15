package builder_test

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/furkanmeclis/go-ubltr"
	"github.com/furkanmeclis/go-ubltr/builder"
	"github.com/furkanmeclis/go-ubltr/codelist"
	"github.com/furkanmeclis/go-ubltr/schema/ubltr121/invoice"
)

var update = flag.Bool("update", false, "rewrite testdata/valid golden files from the builders")

const goldenDir = "../testdata/valid"

// goldenXSLT is a minimal stylesheet embedded in one fixture.
const goldenXSLT = `<?xml version="1.0" encoding="UTF-8"?>
<xsl:stylesheet version="1.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:template match="/"><html><body>Fatura</body></html></xsl:template>
</xsl:stylesheet>
`

type goldenCase struct {
	file    string
	docType string
	build   func(t *testing.T) (*invoice.Invoice, error)
}

func goldenCases() []goldenCase {
	return []goldenCase{
		{"temelfatura-satis.xml", ubltr.TypeEFatura, goldenTemelSatis},
		{"ticarifatura-satis.xml", ubltr.TypeEFatura, goldenTicariSatis},
		{"earsivfatura-satis.xml", ubltr.TypeEArchive, goldenEArsivSatis},
		{"temelfatura-tevkifat.xml", ubltr.TypeEFatura, goldenTevkifat},
		{"temelfatura-istisna.xml", ubltr.TypeEFatura, goldenIstisna},
		{"temelfatura-iade.xml", ubltr.TypeEFatura, goldenIade},
		{"ihracat-istisna.xml", ubltr.TypeEFatura, goldenIhracat},
		{"ticarifatura-usd.xml", ubltr.TypeEFatura, goldenTicariUSD},
	}
}

func TestGoldenInvoices(t *testing.T) {
	t.Parallel()

	for _, tc := range goldenCases() {
		t.Run(tc.file, func(t *testing.T) {
			t.Parallel()

			inv, err := tc.build(t)
			if err != nil {
				t.Fatalf("Build: %v", err)
			}
			data, err := ubltr.Marshal(inv)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			if err := ubltr.ValidateXSD(t.Context(), data); err != nil {
				t.Fatalf("ValidateXSD: %v\n%s", err, data)
			}
			if err := ubltr.ValidateSchematronType(t.Context(), data, tc.docType); err != nil {
				t.Fatalf("ValidateSchematronType(%s): %v\n%s", tc.docType, err, data)
			}

			path := filepath.Join(goldenDir, tc.file)
			if *update {
				if err := os.MkdirAll(goldenDir, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, data, 0o600); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read golden (run go test ./builder -run TestGoldenInvoices -update): %v", err)
			}
			if !bytes.Equal(data, want) {
				t.Fatalf("%s differs from builder output; rerun with -update and review the diff", path)
			}
		})
	}
}

func mustMoney(t *testing.T, currency, amount string) builder.Money {
	t.Helper()
	m, err := builder.MoneyIn(currency, amount)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func mustQty(t *testing.T, unit, value string) builder.Quantity {
	t.Helper()
	q, err := builder.QuantityIn(unit, value)
	if err != nil {
		t.Fatal(err)
	}
	return q
}

func goldenSupplier() *builder.PartyBuilder {
	return builder.NewPartyBuilder().
		VKN("1288331521").
		Name("AAA Anonim Şirketi").
		TaxOffice("Büyük Mükellefler").
		Email("info@example.com").
		AddressBuilder(turkeyAddress())
}

func goldenCompanyCustomer() *builder.PartyBuilder {
	return builder.NewPartyBuilder().
		VKN("9205121120").
		Name("BBB Limited Şirketi").
		TaxOffice("Çankaya").
		AddressBuilder(builder.NewAddressBuilder().
			Street("Atatürk Bulvarı").
			Building("100").
			District("Çankaya").
			City("Ankara").
			PostalZone("06420").
			Country("Türkiye").
			CountryCode("TR"))
}

func goldenBase(profile, typ, id, uuid string) *builder.InvoiceBuilder {
	return builder.NewInvoiceBuilder().
		Profile(profile).
		Type(typ).
		ID(id).
		UUID(uuid).
		IssueDate("2026-03-01").
		IssueTime("10:00:00")
}

func goldenLine(t *testing.T, currency, name, qty, price string, tax *builder.TaxBuilder) *builder.InvoiceLineBuilder {
	t.Helper()
	return builder.NewInvoiceLineBuilder().
		Name(name).
		Quantity(mustQty(t, codelist.UnitC62, qty)).
		Price(mustMoney(t, currency, price)).
		Tax(tax)
}

func goldenTemelSatis(t *testing.T) (*invoice.Invoice, error) {
	return goldenBase(codelist.ProfileTemelFatura, codelist.InvoiceTypeSatis, "GIB2026000000001", "6A2C1D2E-3F40-4B51-8C62-7D8E9FA0B1C2").
		SupplierBuilder(goldenSupplier()).
		CustomerBuilder(personCustomer(t)).
		Line(goldenLine(t, codelist.TRY, "Masa Lambası", "2", "150.00", builder.NewTaxBuilder().KDV("20"))).
		Line(goldenLine(t, codelist.TRY, "Kitap", "3", "40.00", builder.NewTaxBuilder().KDV("10"))).
		Build()
}

func goldenTicariSatis(t *testing.T) (*invoice.Invoice, error) {
	return goldenBase(codelist.ProfileTicariFatura, codelist.InvoiceTypeSatis, "GIB2026000000002", "7B3D2E3F-4051-4C62-9D73-8E9FA0B1C2D3").
		SupplierBuilder(goldenSupplier()).
		CustomerBuilder(goldenCompanyCustomer()).
		OrderReference("SIP-2026-0042", "2026-02-20").
		DespatchReference("IRS2026000000007", "2026-02-28").
		Payment(builder.NewPaymentBuilder().
			Code(codelist.PaymentMeansCreditTransfer).
			DueDate("2026-03-31").
			IBAN("TR330006100519786457841326")).
		Line(goldenLine(t, codelist.TRY, "Ofis Koltuğu", "4", "1250.00", builder.NewTaxBuilder().KDV("20")).
			AllowancePercent("10", "Kampanya iskontosu")).
		AmountInWordsNote().
		XSLT("fatura.xslt", []byte(goldenXSLT)).
		Build()
}

func goldenEArsivSatis(t *testing.T) (*invoice.Invoice, error) {
	return goldenBase(codelist.ProfileEArsivFatura, codelist.InvoiceTypeSatis, "EAR2026000000001", "8C4E3F40-5162-4D73-AE84-9FA0B1C2D3E4").
		SupplierBuilder(goldenSupplier()).
		CustomerBuilder(personCustomer(t)).
		Line(goldenLine(t, codelist.TRY, "Kablosuz Kulaklık", "1", "899.90", builder.NewTaxBuilder().KDV("20"))).
		AmountInWordsNote().
		Build()
}

func goldenTevkifat(t *testing.T) (*invoice.Invoice, error) {
	return goldenBase(codelist.ProfileTemelFatura, codelist.InvoiceTypeTevkifat, "GIB2026000000003", "9D5F4051-6273-4E84-BF95-A0B1C2D3E4F5").
		SupplierBuilder(goldenSupplier()).
		CustomerBuilder(goldenCompanyCustomer()).
		Line(goldenLine(t, codelist.TRY, "Bina Bakım Onarım Hizmeti", "1", "10000.00",
			builder.NewTaxBuilder().KDV("20").Withholding(codelist.Withholding603, "70"))).
		Line(goldenLine(t, codelist.TRY, "Yük Taşımacılığı", "2", "2500.00",
			builder.NewTaxBuilder().KDV("20").Withholding(codelist.Withholding624, "20"))).
		Build()
}

func goldenIstisna(t *testing.T) (*invoice.Invoice, error) {
	return goldenBase(codelist.ProfileTemelFatura, codelist.InvoiceTypeIstisna, "GIB2026000000004", "AE605162-7384-4F95-80A6-B1C2D3E4F506").
		SupplierBuilder(goldenSupplier()).
		CustomerBuilder(goldenCompanyCustomer()).
		Line(goldenLine(t, codelist.TRY, "İstisna Kapsamında Teslim", "5", "200.00",
			builder.NewTaxBuilder().KDV("0").Exemption(codelist.Exemption350, "Diğer istisnalar"))).
		Build()
}

func goldenIade(t *testing.T) (*invoice.Invoice, error) {
	orig, err := goldenTicariSatis(t)
	if err != nil {
		return nil, err
	}
	return builder.NewReturnInvoiceFrom(orig).
		ID("GIB2026000000005").
		UUID("BF716273-8495-4A06-91B7-C2D3E4F50617").
		IssueDate("2026-03-10").
		IssueTime("09:30:00").
		Build()
}

func goldenIhracat(t *testing.T) (*invoice.Invoice, error) {
	foreign := builder.NewAddressBuilder().
		Street("Hauptstraße").
		Building("5").
		District("Mitte").
		City("Berlin").
		PostalZone("10115").
		Country("Almanya").
		CountryCode("DE")
	exempt := func() *builder.TaxBuilder {
		return builder.NewTaxBuilder().KDV("0").Exemption(codelist.Exemption301, "11/1-a Mal ihracatı")
	}
	delivery := func(gtip string) *builder.DeliveryBuilder {
		return builder.NewDeliveryBuilder().
			AddressBuilder(foreign).
			Incoterms(codelist.DeliveryTermFOB).
			CustomsID(gtip).
			TransportMode(codelist.TransportMode1).
			Package("1", codelist.PackageBX, "10")
	}
	return goldenBase(codelist.ProfileIhracat, codelist.InvoiceTypeIstisna, "GIB2026000000006", "C0827384-95A6-4B17-A2C8-D3E4F5061728").
		Currency(codelist.EUR).
		ExchangeRate(codelist.EUR, codelist.TRY, "37.512300", "2026-03-01").
		SupplierBuilder(goldenSupplier()).
		CustomerBuilder(builder.NewExportCustomerBuilder()).
		BuyerCustomerBuilder(builder.NewExportBuyerBuilder().
			Name("Example Handels GmbH").
			LegalEntity("Example Handels GmbH", "DE123456789").
			AddressBuilder(foreign)).
		Line(goldenLine(t, codelist.EUR, "Pamuklu Tişört", "500", "4.50", exempt()).
			Delivery(delivery("610910000011"))).
		Line(goldenLine(t, codelist.EUR, "Kot Pantolon", "200", "12.00", exempt()).
			Delivery(delivery("620342310000"))).
		Build()
}

func goldenTicariUSD(t *testing.T) (*invoice.Invoice, error) {
	return goldenBase(codelist.ProfileTicariFatura, codelist.InvoiceTypeSatis, "GIB2026000000007", "D1938495-A6B7-4C28-B3D9-E4F506172839").
		Currency(codelist.USD).
		ExchangeRate(codelist.USD, codelist.TRY, "34.5678", "2026-03-01").
		SupplierBuilder(goldenSupplier()).
		CustomerBuilder(goldenCompanyCustomer()).
		Line(goldenLine(t, codelist.USD, "Yazılım Lisansı", "10", "99.99", builder.NewTaxBuilder().KDV("20"))).
		AmountInWordsNote().
		Build()
}
