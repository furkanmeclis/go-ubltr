# Builder

Package [`builder`](../builder/) fills generated UBL-TR document types. It does not replace `schema/ubltr121`. Package [`codelist`](../codelist/) holds the official GİB code values.

```
Developer
    ↓
builder + codelist
    ↓
generated Invoice / CreditNote / … + cac / cbc
    ↓
ubltr.Marshal → XML
```

XSD remains the source of truth. Invoice `Build()` returns `*invoice.Invoice`. Credit note `Build()` returns `*creditnote.CreditNote`. The e-Arşiv report builder returns `*earchive.EArsivRaporu` (not an Invoice).

## Invoice

```go
import (
	"github.com/furkanmeclis/go-ubltr"
	"github.com/furkanmeclis/go-ubltr/builder"
	"github.com/furkanmeclis/go-ubltr/codelist"
)

price, err := builder.MoneyIn(codelist.TRY, "10.00")
qty, err := builder.QuantityIn(codelist.UnitC62, "2")

addr := builder.NewAddressBuilder().
	Street("Papatya Caddesi").
	Building("21").
	District("Beşiktaş").
	City("İstanbul").
	PostalZone("34100").
	Country("Türkiye").
	CountryCode("TR")

inv, err := builder.NewInvoiceBuilder().
	Profile(codelist.ProfileTemelFatura).
	Type(codelist.InvoiceTypeSatis).
	ID("GIB20260000000001").
	IssueDate("2026-03-01").
	IssueTime("10:00:00").
	Currency(codelist.TRY).
	SupplierBuilder(builder.NewPartyBuilder().
		VKN("1288331521").
		Name("AAA Anonim Şirketi").
		TaxOffice("Büyük Mükellefler").
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

data, err := ubltr.Marshal(inv)
err = ubltr.ValidateXSD(ctx, data)
```

Required on `Build()`: profile, type, ID, issue date, supplier, customer, at least one line. Currency defaults to `TRY`. UUID is generated when omitted. Line IDs become `1`, `2`, … when empty.

`Build()` also writes UBL 2.1 / TR1.2, `CopyIndicator`, `LineCountNumeric`, a placeholder `UBLExtensions` child (XSD requires one element outside the ext namespace), and a `Signature` block copied from the supplier (not a real XAdES signature).

Builder errors wrap `builder.ErrBuilder` and name a field (`supplier`, `line`, …). They run before XML. They do not replace `ValidateXSD` or `ValidateSchematron`.

## Nested builders

| Constructor | Result |
| --- | --- |
| `NewAddressBuilder` | `cac.AddressType` — city, district (`CitySubdivisionName`), country required |
| `NewPartyBuilder` | `cac.PartyType` — `VKN` / `TCKN` / `Identification`, address required |
| `NewInvoiceLineBuilder` | `cac.InvoiceLineType` — name, quantity, price required |
| `NewTaxBuilder` | applied on the line extension; `KDV("20")` is tax type `0015` |
| `NewPaymentBuilder` | `PaymentMeans` and/or `PaymentTerms` via `InvoiceBuilder.Payment` |
| `NewDeliveryBuilder` | optional delivery date, tracking, address |

You can pass an already-built `cac.PartyType` with `Supplier` / `Customer` instead of `SupplierBuilder` / `CustomerBuilder`.

Line and document discounts:

```go
line.Allowance(amount, "iskonto")
line.AllowancePercent("10", "iskonto")
line.Charge(amount, "nakliye")

inv.Allowance(amount, "belge iskontosu")
inv.Charge(amount, "belge masrafı")
```

Payment:

```go
inv.Payment(builder.NewPaymentBuilder().
	Code(codelist.PaymentMeansCreditTransfer).
	DueDate("2026-03-20").
	IBAN("TR330006100519786457841326").
	Note("havale"))
```

## Other documents

Same party, money, and tax helpers. `Build()` still returns generated types.

| Constructor | Result |
| --- | --- |
| `NewCreditNoteBuilder` + `NewCreditNoteLineBuilder` | `*creditnote.CreditNote` — CreditedQuantity, same totals as Invoice |
| `NewApplicationResponseBuilder` | sender, receiver, `DocumentResponse(id, date, code)` |
| `NewDespatchAdviceBuilder` + `NewDespatchLineBuilder` | required IssueTime, ShipmentID, OrderLineReference |
| `NewReceiptAdviceBuilder` + `NewReceiptLineBuilder` | required `DespatchReference` |

```go
cn, err := builder.NewCreditNoteBuilder().
	Profile(codelist.ProfileTemelFatura).
	Type(codelist.InvoiceTypeIade).
	ID("GIB20260000000002").
	IssueDate("2026-03-02").
	SupplierBuilder(supplier).
	CustomerBuilder(customer).
	Line(builder.NewCreditNoteLineBuilder().
		Name("Widget").
		Quantity(qty).
		Price(price).
		Tax(builder.NewTaxBuilder().KDV("20"))).
	Build()

cn, err = ubltr.NewCreditNoteFromInvoice(inv)
```

## Money and totals

Do not use `float64` for amounts. `builder.Money` wraps `shopspring/decimal`. Currencies must match on `Add` / `Sub`.

```go
price, err := builder.MoneyIn(codelist.TRY, "0.15") // extra scale kept on unit price
qty, err := builder.QuantityIn(codelist.UnitKWH, "101")
ext := price.TimesQuantity(qty).Round() // "15.15"
tax, err := ext.PercentOf("18")
tax.Round().Lexical()                   // "2.73"
```

`ScaleFor` uses ISO 4217 minor units (TRY: 2, JPY: 0, KWD: 3; unknown codes: 2). Totals and tax amounts are written with that many fraction digits. Unit prices keep the scale you parsed.

Automatic document totals (unless `ManualTotals()`):

- Line extension = quantity × price, then line allowance/charge, then round
- Line tax is computed on that extension
- `LegalMonetaryTotal.LineExtensionAmount` = sum of line extensions
- Document allowance/charge adjusts tax-exclusive amount
- Document `TaxTotal` = line taxes grouped by type code and percent
- Payable = tax-inclusive (no prepaid field in the GİB subset)

Document-level allowances do not recompute line KDV. Put rate-affecting discounts on the line.

## e-Arşiv report

`NewEArchiveReportBuilder` fills `*earchive.EArsivRaporu` (`http://earsiv.efatura.gov.tr`). That is the report sent to GİB, not an e-Arşiv invoice.

```go
rep, err := builder.NewEArchiveReportBuilder().
	Version("1.0").
	MukellefVKN("1288331521").
	HazirlayanVKN("1288331521").
	Period("2026-03-01", "2026-03-31").
	Section("2026-03-01", "2026-03-31", "1").
	SignatureInnerXML(dsSignatureInner).
	Invoice(earchive.Fatura{
		FaturaNo:      "GIB20260000000001",
		FaturaTip:     earchive.FaturaTipYTBTEVKIFAT,
		GonderimSekli: earchive.GonderimSekliELEKTRONIK,
		// … remaining required fatura fields
	}).
	Build()
```

`SignatureInnerXML` is the inner XML of `ds:Signature`. This library does not generate XAdES.

## Code lists

UBL XSD does not enumerate unit, currency, or profile codes. The GİB lists are generated from [`UBL-TR_Codelist.xml`](../schemas/ubltr-1.2.1/schematron/UBL-TR_Codelist.xml) into `codelist`. Do not edit `codelist/generated.go` or `codelist/lookup.go`.

```go
codelist.UnitC62                 // "C62"
codelist.UnitKWH                 // "KWH"
codelist.ValidUnit("KWH")        // every GİB unit (2000+)
codelist.ValidCurrency("TRY")
codelist.ProfileTemelFatura      // "TEMELFATURA"
codelist.InvoiceTypeSatis        // "SATIS"
codelist.TaxKDV                  // "0015"
codelist.PaymentMeansCreditTransfer
```

`Valid*` functions exist per list (`ValidProfile`, `ValidInvoiceType`, `ValidTax`, `ValidMIME`, `ValidPackage`, …). MIME codes are lookup-only (slashes are not valid Go identifiers).

Regenerate with `make generate` after updating the vendored GİB package.

## Tests

`go test ./builder/...` builds sample Invoice, CreditNote, ApplicationResponse, DespatchAdvice, ReceiptAdvice, and e-Arşiv report documents. UBL samples run `ValidateXSD`. `go test ./codelist/...` checks generated lists against the XML source.
