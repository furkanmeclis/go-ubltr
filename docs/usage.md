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

`ubltr.Detect(xmlBytes)` returns `KindInvoice`, `KindCreditNote`, `KindApplicationResponse`, `KindDespatchAdvice`, `KindReceiptAdvice`, `KindEArchiveReport`, or `KindEnvelope` (GİB `sh:StandardBusinessDocument`) from the root element. It does not look at ProfileID. `KindEArchiveReport` is `eArsivRaporu`, not an e-Arşiv Invoice.

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

```go
err = ubltr.UnmarshalReader(r, &loaded)
err = ubltr.MarshalWrite(w, &inv)
```

Official sample XML still unmarshals. Extra `xmlns` on GİB fixtures (for example `n4` in `ExtensionContent`) is not round-tripped on marshal.

## Clone, inspect, attachments, diff

```go
clone, err := ubltr.CloneInvoice(&inv)
cn, err := ubltr.NewCreditNoteFromInvoice(&inv)

sum, err := ubltr.Inspect(xmlBytes)
fmt.Println(sum.String())

err = ubltr.AddAttachment(&inv, ubltr.Attachment{
	ID:       "ATT-1",
	MIME:     "application/pdf",
	Filename: "note.pdf",
	Data:     pdfBytes,
})

changes, err := ubltr.Diff(xmlA, xmlB)
```

`CloneInvoice` / `CloneCreditNote` deep-copy via marshal/unmarshal; the copy does not share slice or pointer identity with the source.

`Inspect` uses `Detect` and unmarshal. It does not run XSD or Schematron. Invoice summaries include ID, profile, parties, line count, tax, and payable.

`AddAttachment` writes `AdditionalDocumentReference` with base64 `EmbeddedDocumentBinaryObject`. MIME must be a GİB MimeCodeList value. UBL-TR allows `mimeCode` only on that binary object; `Filename` is stored as `DocumentDescription` when `Description` is empty.

`Diff` compares XML element trees (developer tool, not a GİB checker). Namespace prefixes are ignored.

## Validation

```go
err = ubltr.ValidateXSD(ctx, xmlBytes)                            // schema for Detect(xmlBytes)
err = ubltr.ValidateSchematron(ctx, xmlBytes)                     // type=efatura; rules for the detected kind
err = ubltr.ValidateSchematronType(ctx, xmlBytes, ubltr.TypeEArchive)
err = ubltr.Validate(ctx, xmlBytes)                               // XSD, then Schematron where HasSchematron
```

| Function | Meaning |
| --- | --- |
| `ValidateXSD` | GİB XSD for the detected document kind (UBL-TR, e-Arşiv report, or envelope) |
| `ValidateSchematron` | GİB business rules for the detected kind, `efatura` |
| `ValidateSchematronType` | Same rules with `efatura`, `earchive`, or `goruntuleme` |
| `HasSchematron` | Whether the GİB Main Schematron has rules for a kind |
| `Validate` | XSD first, then Schematron when `HasSchematron(kind)` |

Schematron coverage follows the vendored `UBL-TR_Main_Schematron.xml`:

| Kind | Patterns run |
| --- | --- |
| `KindInvoice` | `invoice` + `general` |
| `KindDespatchAdvice` (e-İrsaliye) | `despatchadvice` + `general` |
| `KindReceiptAdvice` | `receiptadvice` + `general` |
| `KindApplicationResponse` | `applicationresponse` + `general` |
| `KindEnvelope` | every pattern (header, package, and the documents inside `ef:Package`) |
| `KindCreditNote`, `KindEArchiveReport` | none; `Validate` checks XSD only |

`general` holds the GİB rules outside any pattern (currency, unit, country, MIME, and channel code lists, `IssueDate` checks). The GİB `type` parameter only affects ProfileID-related checks: `efatura` (default) checks invoices against the e-Fatura profiles and despatch advices against the e-İrsaliye profiles; `earchive` requires `EARSIVFATURA` and skips the despatch profile check; `goruntuleme` (viewing) accepts any e-Fatura profile or `EARSIVFATURA` and the e-İrsaliye profiles.

XSD success is not GİB business-rule success. The 2009 files in `testdata/ubltr-1.2.1/` pass XSD and fail current Schematron (for example customer TCKN length).

### Typed failures

```go
var xe *ubltr.XSDValidationError
var se *ubltr.SchematronValidationError
if errors.As(err, &xe) { /* xe.Violations, xe.Messages, xe.Errors(), xe.Warnings() */ }
if errors.As(err, &se) { /* se.Violations, se.Messages, se.Errors(), se.Warnings() */ }

for _, v := range ubltr.Violations(err) { // works through fmt.Errorf("%w") wrapping
	fmt.Println(v.Line, v.PatternID, v.RuleID, v.Location, v.Message)
}
```

Malformed XML is a wrapped parse error, not those types; `Violations` returns nil for it.

Each `Violation` has:

| Field | XSD | Schematron |
| --- | --- | --- |
| `Layer` | `xsd` | `schematron` |
| `Severity` | `error` or `warning` (helium level) | `error` for `assert`, `warning` for `report`; a `role` or `flag` of `fatal`/`error`, `warning`/`warn`, or `info`/`information` overrides it |
| `Message` | helium message | GİB message (Turkish) with `value-of` filled in |
| `RuleID` | — | rule id; GİB rules are abstract rules such as `ProfileIDCheck` pulled in with `sch:extends` |
| `TestID` | — | `id` of the failing `assert` / `report` (GİB asserts have none) |
| `PatternID` | — | `invoice`, `despatchadvice`, `applicationresponse`, `receiptadvice`, `header`, `package`, … or `general` |
| `Context`, `Test` | — | rule context and failing XPath test, as written in the schema |
| `Role`, `Flag` | — | raw Schematron attributes, if any |
| `Location` | path of the element on the reported line | path of the context node |
| `Line` | source line | source line of the context node (owner element for attributes) |
| `Element`, `Attribute` | failing element / attribute | context element / attribute |
| `Path` | `element:line` (kept for compatibility) | element name as written (kept for compatibility) |

`Location` is XPath-like and uses the GİB Schematron prefixes whatever prefixes the document uses: `inv`, `apr`, `desp`, `recp`, `sh`, `ef`, `cac`, `cbc`, `ext`, `ds`, `xades` (plus `cn` and `earsiv`). A position predicate appears only when siblings share a name, for example `/inv:Invoice/cac:InvoiceLine[2]/cbc:InvoicedQuantity/@unitCode`. Unknown namespaces are written as `Q{uri}local`.

`Violation` has JSON tags (`layer`, `ruleId`, `patternId`, `location`, `line`, …); `ubltr validate --format=json` prints them. Any violation, including a warning, makes the validator return an error; use `Errors()` to decide whether to reject. The vendored GİB Schematron only has asserts, so every GİB violation is an error today.

## Envelopes

GİB sends documents inside a Standard Business Document envelope (`sh:StandardBusinessDocument`): a header with sender and receiver aliases and VKN/TCKN, and an `ef:Package` with `Elements` groups of documents. Types are generated in `schema/envelope`.

```go
env, err := ubltr.NewEnvelope(ubltr.EnvelopeOptions{
	Sender:   ubltr.EnvelopeParty{Alias: "urn:mail:defaultgb@example.com", VKNTCKN: "1288331521", Name: "AAA A.Ş."},
	Receiver: ubltr.EnvelopeParty{Alias: "urn:mail:defaultpk@example.com", VKNTCKN: "12345678901"},
}, inv1, inv2, signedInvoiceBytes)
data, err := ubltr.Marshal(env) // *envelope.StandardBusinessDocument
err = ubltr.Validate(ctx, data) // envelope XSD (and the documents inside), then every GİB pattern

sbd, docs, err := ubltr.UnmarshalEnvelope(data) // *envelope.StandardBusinessDocument, []ubltr.EnvelopeDocument
for _, d := range docs {
	inv, ok := d.Document.(*invoice.Invoice) // also d.Kind, d.ElementType, d.XML
	_, _ = inv, ok
}
docs, err = ubltr.EnvelopeDocuments(data)
```

`NewEnvelope` accepts pointers to generated documents (`*invoice.Invoice`, `*creditnote.CreditNote`, `*despatchadvice.DespatchAdvice`, `*applicationresponse.ApplicationResponse`, `*receiptadvice.ReceiptAdvice`) or `[]byte` of an already serialized document (kept byte for byte, so signatures survive). Documents are grouped into one `Elements` per `ElementType` in first-seen order. Defaults: `Type` is `SENDERENVELOPE` (or `POSTBOXENVELOPE` when the first document is an ApplicationResponse or ReceiptAdvice), a random `InstanceIdentifier`, the current time, `HeaderVersion` `1.0`, `TypeVersion` `1.2`, `Standard` `UBL-TR`, and the `xsi:schemaLocation` GİB Schematron expects. The VKN/TCKN must match the document parties, or GİB rules such as `DocumentSenderCheck` fail.

`EnvelopeDocuments` decodes with the envelope's namespace context, so prefixes declared on the envelope root work. `EnvelopeDocument.XML` is the original document bytes when they parse on their own; otherwise the typed document is marshaled again. `ElementList` itself is `InnerXML` in the generated types.

Envelope XSD validation resolves the GİB `Package_1_2.xsd` imports of the UBL maindoc schemas from the vendored files without editing them. The HR-XML user-account schema is not vendored; its import is answered with an empty schema, so `PROCESSUSERACCOUNT` / `CANCELUSERACCOUNT` packages do not validate.

Full programs: root [README](../README.md). HTML and PDF: [rendering.md](rendering.md). CLI: [cli.md](cli.md).
