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
	ID("GIB2026000000001").
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
| `NewDeliveryBuilder` | delivery date, tracking, address, INCOTERMS, GTİP, transport mode, packages |
| `NewExportCustomerBuilder` | IHRACAT `AccountingCustomerParty` (GTB, VKN `1460415308`) |
| `NewExportBuyerBuilder` | IHRACAT `BuyerCustomerParty` with `PARTYTYPE=EXPORT` |
| `NewTaxFreeBuyerBuilder` | YOLCUBERABERFATURA `BuyerCustomerParty` with `PARTYTYPE=TAXFREE` |

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

References:

```go
inv.OrderReference("SIP-2026-0042", "2026-02-20")        // cac:OrderReference
inv.DespatchReference("IRS2026000000007", "2026-02-28") // cac:DespatchDocumentReference
inv.BillingReference("GIB2026000000002", "2026-03-01")  // cac:BillingReference/InvoiceDocumentReference
inv.AdditionalDocument("SOZ-1", "", "", "Sözleşme")      // cac:AdditionalDocumentReference (date defaults to IssueDate)

ref, err := ubltr.NewAttachmentReference(ubltr.Attachment{ /* … */ })
inv.AdditionalDocumentReference(ref) // embedded file
```

## Withholding (tevkifat)

`TaxBuilder.Withholding(code, percent)` marks part of the line KDV as withheld by the buyer. `code` is a GİB `WithholdingTaxType` value (`codelist.Withholding603`, …) and the code/percent pair must be in `WithholdingTaxTypeWithPercent` (`603` + `70`, `624` + `20`, `801` + `100`, …); other pairs fail on the line. `"70.00"` is written as `70`.

```go
inv, err := builder.NewInvoiceBuilder().
	Profile(codelist.ProfileTemelFatura).
	Type(codelist.InvoiceTypeTevkifat).
	// … ID, dates, parties
	Line(builder.NewInvoiceLineBuilder().
		Name("Bina Bakım Onarım Hizmeti").
		Quantity(qty).
		Price(price).
		Tax(builder.NewTaxBuilder().KDV("20").Withholding(codelist.Withholding603, "70"))).
	Build()
```

- Line `cac:WithholdingTaxTotal`: TaxableAmount = line KDV, TaxAmount = KDV × percent / 100, TaxScheme `KDV TEVKİFAT` (`WithholdingName` to override) with the withholding code.
- Document `cac:WithholdingTaxTotal`: line withholdings grouped by code and percent.
- `TaxInclusiveAmount` stays tax-exclusive + full KDV; `PayableAmount` = TaxInclusive − withheld.
- `Build()` fails when withholding is used on a type that does not allow it (allowed: TEVKIFAT, YTBTEVKIFAT, IADE, YTBIADE, TEVKIFATIADE, YTBTEVKIFATIADE, SGK, SARJ, SARJANLIK) and when TEVKIFAT / YTBTEVKIFAT has no withholding.
- Credit note lines have no `WithholdingTaxTotal`; a withholding tax there fails.

The vendored GİB Schematron (`GeneralWithholdingTaxTotalCheck`) rejects `WithholdingTaxTotal` on TEVKIFATIADE and YTBTEVKIFATIADE. Use IADE / YTBIADE for withholding returns until GİB updates that rule.

## Exemptions

`TaxBuilder.Exemption(code, reason)` checks the code against `TaxExemptionReasonCodeType`. Document `TaxTotal` keeps the exemption code and reason (subtotals are grouped by tax code, percent, and exemption code), which GİB requires for zero KDV:

```go
builder.NewTaxBuilder().KDV("0").Exemption(codelist.Exemption350, "Diğer istisnalar")    // type ISTISNA
builder.NewTaxBuilder().KDV("0").Exemption(codelist.Exemption301, "11/1-a Mal ihracatı") // IHRACAT
```

## Return invoices (İade)

A return in Türkiye is an Invoice with InvoiceTypeCode IADE (or TEVKIFATIADE, YTBIADE, YTBTEVKIFATIADE) and one `cac:BillingReference/cac:InvoiceDocumentReference` per returned invoice, with `DocumentTypeCode` `IADE` and the 16-character invoice ID. `BillingReference` writes that code automatically on return types; `Build()` fails without a reference or with an ID that is not 16 characters.

```go
inv.Type(codelist.InvoiceTypeIade).ReturnOf(orig) // or BillingReference(id, date)

ret, err := builder.NewReturnInvoiceFrom(orig).
	ID("GIB2026000000005").
	IssueDate("2026-03-10").
	Build()
```

`NewReturnInvoiceFrom` swaps the parties (the original customer issues the return), copies lines (with their KDV and withholding), currency, and exchange rate, sets type IADE (YTBIADE for YTB invoices), switches the profile to TEMELFATURA when the original profile does not allow IADE (TICARIFATURA, IHRACAT, …), and adds the billing reference. A new UUID is generated on Build. Options: `ReturnType(code)`, `ReturnLines("1", "3")` for a partial return, `ReturnSameParties()`.

## Foreign currency

GİB requires `cac:PricingExchangeRate` when `DocumentCurrencyCode` is not TRY. `Build()` fails without it, or when its source currency is not the document currency. The rate may have at most 6 fraction digits.

```go
inv.Currency(codelist.USD).
	ExchangeRate(codelist.USD, codelist.TRY, "34.5678", "2026-03-01")
```

## Export (IHRACAT)

```go
foreign := builder.NewAddressBuilder().City("Berlin").District("Mitte").Country("Almanya").CountryCode("DE")

inv, err := builder.NewInvoiceBuilder().
	Profile(codelist.ProfileIhracat).
	Type(codelist.InvoiceTypeIstisna).
	Currency(codelist.EUR).
	ExchangeRate(codelist.EUR, codelist.TRY, "37.512300", "2026-03-01").
	SupplierBuilder(supplier). // TaxOffice is required on IHRACAT
	CustomerBuilder(builder.NewExportCustomerBuilder()).
	BuyerCustomerBuilder(builder.NewExportBuyerBuilder().
		Name("Example Handels GmbH").
		LegalEntity("Example Handels GmbH", "DE123456789").
		AddressBuilder(foreign)).
	Line(builder.NewInvoiceLineBuilder().
		Name("Pamuklu Tişört").
		Quantity(qty).
		Price(price).
		Tax(builder.NewTaxBuilder().KDV("0").Exemption(codelist.Exemption301, "11/1-a Mal ihracatı")).
		Delivery(builder.NewDeliveryBuilder().
			AddressBuilder(foreign).
			Incoterms(codelist.DeliveryTermFOB).       // DeliveryTerms/ID schemeID="INCOTERMS"
			CustomsID("610910000011").                 // Shipment/GoodsItem/RequiredCustomsID (GTİP)
			TransportMode(codelist.TransportMode1).    // Shipment/ShipmentStage/TransportModeCode
			Package("1", codelist.PackageBX, "10"))). // TransportHandlingUnit/ActualPackage
	// … ID, dates
	Build()
```

`Build()` fails on IHRACAT without a buyer or when a line has no GTİP. INCOTERMS, transport mode, and packaging codes are checked against the GİB lists. `TaxRepresentative` / `TaxRepresentativeBuilder` set `cac:TaxRepresentativeParty` (the YOLCUBERABERFATURA intermediary with `ARACIKURUMVKN` / `ARACIKURUMETIKET` identifications). `PartyBuilder.Nationality` sets `Person/NationalityID`.

## Amount in words

```go
builder.AmountInWords(m) // 123.45 TRY → "YALNIZ YÜZ YİRMİ ÜÇ TÜRK LİRASI KIRK BEŞ KURUŞ"
inv.AmountInWordsNote()  // appends the payable amount in words as cbc:Note on Build
```

Units are Turkish for TRY, USD, EUR, GBP, CHF, JPY, RUB, CNY, SAR, AED, and KWD; other currencies use the ISO code. Zero is `SIFIR`, 1000 is `BİN`, and a zero fraction is omitted. `AmountInWordsNote` needs automatic totals.

## Embedded XSLT

```go
inv.XSLT("fatura.xslt", xsltBytes)

ref, err := builder.XSLTReference(built.UUID.Value, built.IssueDate.Value, "fatura.xslt", xsltBytes)
built.AdditionalDocumentReference = append(built.AdditionalDocumentReference, ref)
```

The reference has the invoice UUID as ID, `DocumentType` `XSLT`, and a base64 `EmbeddedDocumentBinaryObject` with `mimeCode="application/xml"`. UBL-TR 1.2.1 `BinaryObjectType` allows only `mimeCode` and `format`, so the filename is written as `DocumentDescription`.

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
- Payable = tax-inclusive − document withholding (no prepaid field in the GİB subset)

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
		FaturaNo:      "GIB2026000000001",
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

Golden invoices in `testdata/valid/` (TEMELFATURA SATIS, TICARIFATURA SATIS, EARSIVFATURA SATIS, TEMELFATURA TEVKIFAT, TEMELFATURA ISTISNA, TEMELFATURA IADE, IHRACAT ISTISNA, TICARIFATURA in USD) are built with fixed UUIDs and dates and must pass `ValidateXSD` and `ValidateSchematronType` (`efatura`, or `earchive` for EARSIVFATURA). Regenerate after an intended builder change:

```bash
go test ./builder -run TestGoldenInvoices -update
```
