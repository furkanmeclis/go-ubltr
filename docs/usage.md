# Usage

Package `ubltr` works on `[]byte` XML and generated document structs. Fill the struct, unmarshal existing XML, or construct with [`builder`](builder.md).

## Types

```go
import (
	"github.com/furkanmeclis/go-ubltr/schema/ubltr121/creditnote"
	"github.com/furkanmeclis/go-ubltr/schema/ubltr121/invoice"
)

inv := ubltr.NewInvoice()
cn := ubltr.NewCreditNote()
rep := ubltr.NewEArchiveReport()
var loaded invoice.Invoice
```

`ubltr.Detect(xmlBytes)` returns `KindInvoice`, `KindCreditNote`, `KindApplicationResponse`, `KindDespatchAdvice`, `KindReceiptAdvice`, or `KindEArchiveReport` from the root element. It does not look at ProfileID. `KindEArchiveReport` is `eArsivRaporu`, not an e-Arşiv Invoice.

`ubltr.NewCreditNoteFromInvoice(inv)` copies parties, lines (quantity → CreditedQuantity), totals, and a BillingReference to the original invoice. It writes a new UUID and a placeholder `UBLExtensions` child (GİB sample `n4:` wildcards are not round-tripped).

Amounts, dates, and times are `string` so lexical values survive round-trip. Identifiers keep `schemeID`.

## Builder

Fluent Invoice construction, decimal money, and GİB code lists: [builder.md](builder.md).

## Marshal and unmarshal

```go
data, err := ubltr.Marshal(&inv)
err = ubltr.Unmarshal(xmlBytes, &loaded)
```

`Marshal` writes an XML declaration. `encoding/xml` emits namespace URIs; `cbc:` / `cac:` prefixes are not guaranteed. That is valid XML.

Official sample XML still unmarshals. Extra `xmlns` on GİB fixtures (for example `n4` in `ExtensionContent`) is not round-tripped on marshal.

## Validation

```go
err = ubltr.ValidateXSD(ctx, xmlBytes)                            // schema for Detect(xmlBytes)
err = ubltr.ValidateSchematron(ctx, xmlBytes)                     // type=efatura; Invoice rules
err = ubltr.ValidateSchematronType(ctx, xmlBytes, ubltr.TypeEArchive)
err = ubltr.Validate(ctx, xmlBytes)                               // XSD; Schematron only for Invoice
```

| Function | Meaning |
| --- | --- |
| `ValidateXSD` | GİB XSD for the detected document kind (UBL-TR or e-Arşiv report) |
| `ValidateSchematron` | Invoice business rules, `efatura` |
| `ValidateSchematronType` | Same rules with `efatura` or `earchive` |
| `Validate` | XSD first; Schematron only for Invoice |

XSD success is not GİB business-rule success. The 2009 files in `testdata/ubltr-1.2.1/` pass XSD and fail current Schematron (for example customer TCKN length).

Typed failures:

```go
var xe *ubltr.XSDValidationError
var se *ubltr.SchematronValidationError
if errors.As(err, &xe) { /* xe.Violations, xe.Messages */ }
if errors.As(err, &se) { /* se.Violations, se.Messages */ }
```

Malformed XML is a wrapped parse error, not those types.

Full programs: root [README](../README.md). HTML and PDF: [rendering.md](rendering.md).
