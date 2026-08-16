package builder_test

import (
	"fmt"

	"github.com/furkanmeclis/go-ubltr/builder"
	"github.com/furkanmeclis/go-ubltr/codelist"
)

func ExampleNewInvoiceBuilder() {
	price, err := builder.MoneyIn(codelist.TRY, "10.00")
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	qty, err := builder.QuantityIn(codelist.UnitC62, "2")
	if err != nil {
		fmt.Println("error:", err)
		return
	}

	addr := builder.NewAddressBuilder().
		Street("Papatya Caddesi").
		Building("21").
		District("Beşiktaş").
		City("İstanbul").
		PostalZone("34100").
		Country("Türkiye")

	inv, err := builder.NewInvoiceBuilder().
		Profile(codelist.ProfileTemelFatura).
		Type(codelist.InvoiceTypeSatis).
		ID("GIB20260000000001").
		IssueDate("2026-03-01").
		SupplierBuilder(builder.NewPartyBuilder().
			VKN("1288331521").
			Name("AAA Anonim Şirketi").
			AddressBuilder(addr)).
		CustomerBuilder(builder.NewPartyBuilder().
			TCKN("12345678901").
			Person("Ali", "Yılmaz").
			AddressBuilder(addr)).
		Line(builder.NewInvoiceLineBuilder().
			Name("Widget").
			Quantity(qty).
			Price(price).
			Tax(builder.NewTaxBuilder().KDV("20"))).
		Build()
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println(inv.LegalMonetaryTotal.PayableAmount.Value)
	// Output: 24.00
}
