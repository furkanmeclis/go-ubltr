# Usage

Package `ubltr` works on `[]byte` XML and generated Invoice structs. Builders for Party, Tax, and Line are not in this release; fill the struct or unmarshal existing XML.

## Types

```go
import "github.com/furkanmeclis/go-ubltr/schema/ubltr121/invoice"

inv := ubltr.NewInvoice()
var loaded invoice.Invoice
```

Amounts, dates, and times are `string` so lexical values survive round-trip. Identifiers keep `schemeID`.

## Marshal and unmarshal

```go
data, err := ubltr.Marshal(&inv)
err = ubltr.Unmarshal(xmlBytes, &loaded)
```

`Marshal` writes an XML declaration. `encoding/xml` emits namespace URIs; `cbc:` / `cac:` prefixes are not guaranteed. That is valid XML.

Official sample XML still unmarshals. Extra `xmlns` on GİB fixtures (for example `n4` in `ExtensionContent`) is not round-tripped on marshal.

## Validation

```go
err = ubltr.ValidateXSD(ctx, xmlBytes)
err = ubltr.ValidateSchematron(ctx, xmlBytes)                 // type=efatura
err = ubltr.ValidateSchematronType(ctx, xmlBytes, ubltr.TypeEArchive)
err = ubltr.Validate(ctx, xmlBytes)                           // XSD, then e-Fatura Schematron
```

| Function | Meaning |
| --- | --- |
| `ValidateXSD` | GİB UBL-TR 1.2.1 Invoice XSD via helium |
| `ValidateSchematron` | Business rules, `efatura` |
| `ValidateSchematronType` | Same rules with `efatura` or `earchive` |
| `Validate` | XSD first; Schematron only if XSD passes |

XSD success is not GİB business-rule success. The 2009 files in `testdata/ubltr-1.2.1/` pass XSD and fail current Schematron (for example customer TCKN length).

Typed failures:

```go
var xe *ubltr.XSDValidationError
var se *ubltr.SchematronValidationError
if errors.As(err, &xe) { /* xe.Messages */ }
if errors.As(err, &se) { /* se.Messages */ }
```

Malformed XML is a wrapped parse error, not those types.

Full programs: root [README](../README.md). HTML and PDF: [rendering.md](rendering.md).
