# go-ubltr

[![CI](https://github.com/furkanmeclis/go-ubltr/actions/workflows/ci.yml/badge.svg)](https://github.com/furkanmeclis/go-ubltr/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/furkanmeclis/go-ubltr.svg)](https://pkg.go.dev/github.com/furkanmeclis/go-ubltr)
[![License: MIT](https://img.shields.io/github/license/furkanmeclis/go-ubltr)](LICENSE)

Go library for creating, validating, and rendering Turkish Revenue Administration (GİB) UBL-TR electronic-invoice and e-Arşiv Invoice XML. It also covers CreditNote, ApplicationResponse, DespatchAdvice, ReceiptAdvice, and the e-Arşiv *report*.

**UBL-TR schema definitions are the source of truth. Generated Go types are derived from the schemas.**

Do not edit files under `schema/`. Regenerated types come from the vendored XSD in `schemas/` via `ubltr-gen`.

- Documentation: [`docs/`](docs/) ([install](docs/install.md), [usage](docs/usage.md), [builder](docs/builder.md), [rendering](docs/rendering.md), [cli](docs/cli.md), [testing](docs/testing.md))
- [Changelog](CHANGELOG.md)
- [Roadmap](ROADMAP.md)
- [Contributing](CONTRIBUTING.md)
- [Security policy](SECURITY.md)
- [Code of conduct](CODE_OF_CONDUCT.md)

## Install

Requires Go 1.26.1 or later.

```bash
go get github.com/furkanmeclis/go-ubltr
```

That module is XML, XSD, and Schematron only. It does not download Chrome, chromedp, or an XSLT adapter.

HTML rendering:

```bash
go get github.com/furkanmeclis/go-ubltr/render
```

PDF (needs a Chrome/Chromium binary at runtime):

```bash
go get github.com/furkanmeclis/go-ubltr/render/chromium
```

Command-line tool (nested module; `--pdf` may pull chromedp):

```bash
go install github.com/furkanmeclis/go-ubltr/cmd/ubltr@latest
```

From a clone: `go install ./cmd/ubltr`. See [cli](docs/cli.md).

## Usage

```go
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/furkanmeclis/go-ubltr"
	"github.com/furkanmeclis/go-ubltr/schema/ubltr121/invoice"
)

func main() {
	data, err := os.ReadFile("invoice.xml")
	if err != nil {
		panic(err)
	}

	var inv invoice.Invoice
	if err := ubltr.Unmarshal(data, &inv); err != nil {
		panic(err)
	}
	fmt.Println(inv.ID.Value)

	if err := ubltr.ValidateXSD(context.Background(), data); err != nil {
		panic(err)
	}
	if err := ubltr.ValidateSchematron(context.Background(), data); err != nil {
		fmt.Println(err)
	}

	out, err := ubltr.Marshal(&inv)
	if err != nil {
		panic(err)
	}
	_ = out
}
```

`ubltr.NewInvoice()` returns an empty `*invoice.Invoice`. Everyday construction lives in [`builder`](builder/) plus generated GİB lists in [`codelist`](codelist/):

```go
import (
	"github.com/furkanmeclis/go-ubltr"
	"github.com/furkanmeclis/go-ubltr/builder"
	"github.com/furkanmeclis/go-ubltr/codelist"
)

price, err := builder.MoneyIn(codelist.TRY, "10.00")
qty, err := builder.QuantityIn(codelist.UnitC62, "2")
inv, err := builder.NewInvoiceBuilder().
	Profile(codelist.ProfileTemelFatura).
	Type(codelist.InvoiceTypeSatis).
	ID("GIB2026000000001").
	IssueDate("2026-03-01").
	SupplierBuilder(supplier).
	CustomerBuilder(customer).
	Line(builder.NewInvoiceLineBuilder().
		Name("Widget").
		Quantity(qty).
		Price(price).
		Tax(builder.NewTaxBuilder().KDV("20"))).
	Build()
data, err := ubltr.Marshal(inv)
```

`Build()` returns generated types (`*invoice.Invoice`, `*creditnote.CreditNote`, `*earchive.EArsivRaporu`, …). `ubltr.Detect` reads the XML root; `ValidateXSD` uses the matching schema. `NewCreditNoteFromInvoice` copies parties, lines, and totals onto a CreditNote and adds a billing reference. `CloneInvoice`, `Inspect`, `AddAttachment`, and `Diff` operate on those documents; see [usage](docs/usage.md).

Unit, currency, profile, tax, and related codes are the full GİB lists (`codelist.ValidUnit("C62")`, `codelist.UnitKWH`, …), generated from `UBL-TR_Codelist.xml` (UBL XSD does not enumerate them).

## What is generated

`make generate` runs `cmd/ubltr-gen` four times:

```
schemas/ubltr-1.2.1/xsdrt/maindoc/UBL-{Invoice,CreditNote,ApplicationResponse,DespatchAdvice,ReceiptAdvice}-2.1.xsd
        ↓
schema/ubltr121/{invoice,creditnote,applicationresponse,despatchadvice,receiptadvice,cac,cbc,ext,udt}

schemas/earchive/EArsiv.xsd
        ↓
schema/earchive

schemas/ubltr-1.2.1/envelope/PackageProxy_1_2.xsd
        ↓
schema/envelope

schemas/ubltr-1.2.1/schematron/UBL-TR_Codelist.xml
        ↓
codelist/{generated.go,lookup.go}
```

Types reachable from those five UBL document elements are emitted into shared `cac` / `cbc` packages. The GİB envelope (Standard Business Document Header plus `ef:Package`) is generated into `schema/envelope`; documents inside `ElementList` stay raw XML there, and `ubltr.EnvelopeDocuments` decodes them into the UBL types. The e-Arşiv report is a separate schema (`http://earsiv.efatura.gov.tr`, root `eArsivRaporu`); it is not a UBL Invoice. e-Arşiv *invoices* remain UBL Invoice with `ProfileID` `EARSIVFATURA`.

Amounts, dates, and times are `string` so lexical values survive round-trip. Identifier types keep `schemeID`.

## Validation

| Layer | API | Notes |
| --- | --- | --- |
| Kind | `Detect` | Root element + namespace (Invoice, CreditNote, …, e-Arşiv report, envelope). |
| XSD | `ValidateXSD` | [helium](https://github.com/lestrrat-go/helium) against the schema for the detected kind. |
| Schematron | `ValidateSchematron`, `ValidateSchematronType` | Official GİB rules for Invoice, ApplicationResponse, DespatchAdvice, ReceiptAdvice, and the envelope. |
| Both | `Validate` | XSD first, then Schematron where `HasSchematron` is true. |

`ValidateSchematron` uses `type=efatura`. Pass `ubltr.TypeEArchive` to `ValidateSchematronType` for e-Arşiv (`ProfileID` must be `EARSIVFATURA`).

Failures are `*XSDValidationError` / `*SchematronValidationError` with `Violations` that carry rule ID, pattern, XPath-like location, line, and severity; `ubltr.Violations(err)` returns them from any wrapped error.

XSD success is not GİB business-rule success. Official 2009 sample invoices in `testdata/` are XSD-valid and fail current Schematron (for example customer TCKN length). Helium’s native Schematron compiler still cannot ingest the unmodified GİB files; this library flattens them first.

## Rendering

Style (XML → HTML) and PDF engine (HTML → PDF) are separate nested modules. Core does not import them.

```go
import (
	"github.com/furkanmeclis/go-ubltr/render"
	"github.com/furkanmeclis/go-ubltr/render/chromium"
	heliumxslt "github.com/furkanmeclis/go-ubltr/render/xslt/helium"
)

htmlR, err := render.NewHTML(heliumxslt.New(), render.StyleMinimal())
html, err := htmlR.Render(ctx, xmlBytes)

pdfR := chromium.New(ctx)
defer pdfR.Close()
pdf, err := render.ToPDF(ctx, htmlR, pdfR, xmlBytes, render.PDFOptions{})
```

Pass your own XSLT as `render.Style{Name: "company", Stylesheet: xslBytes}`. `.xsl` and `.xslt` are the same format. A GİB `general.xslt` trial lives in `examples/render/general/` (not a core module dependency).

Default XSLT engine is helium `xslt3` (pure Go). `xsi:schemaLocation` on GİB XML is ignored; `document()` and `xsl:include` cannot read the host. Default PDF engine is headless Chrome via chromedp, A4 portrait. Docker images should install a Turkish-capable font such as Noto Sans.

| Need | Module |
| --- | --- |
| XML + XSD + Schematron | `github.com/furkanmeclis/go-ubltr` |
| HTML | `github.com/furkanmeclis/go-ubltr/render` |
| PDF | `github.com/furkanmeclis/go-ubltr/render/chromium` |
| CLI | `github.com/furkanmeclis/go-ubltr/cmd/ubltr` |
| Integrator contract | `github.com/furkanmeclis/go-ubltr/provider` |
| One integrator | `github.com/furkanmeclis/go-ubltr/provider/<name>` |
| Polling to events | `github.com/furkanmeclis/go-ubltr/provider/watch` (+ `/redis`, `/sql`) |

## Integrators (özel entegratör)

Integrator clients are nested modules; the core module never imports them.

```bash
go get github.com/furkanmeclis/go-ubltr/provider/uyumsoft
```

```go
import (
	"github.com/furkanmeclis/go-ubltr/provider"
	"github.com/furkanmeclis/go-ubltr/provider/uyumsoft"
)

c, err := uyumsoft.New(uyumsoft.Config{Username: user, Password: pass, Test: true})
doc := &provider.Document{XML: xmlBytes, ReceiverAlias: "urn:mail:defaultpk@example.com"}
_ = doc.Inspect() // fills kind, UUID, ID, profile, receiver VKN
res, err := c.Send(ctx, doc)
if errors.Is(err, provider.ErrQuotaExceeded) { /* kontör */ }
```

Every module declares what it supports (`Capabilities()`, `provider.Supports`), returns `*provider.Error` values with a normalized kind plus the raw vendor and GİB codes, and keeps its vendor code list in an embedded `errors.csv`. `provider/watch` turns polling into events (`ubltr watch --provider izibiz … | your-handler`) for integrators without webhooks. See [providers](docs/providers.md) and [watch](docs/watch.md).

| Module | Protocol | Integrator |
| --- | --- | --- |
| `provider/uyumsoft` | SOAP | Uyumsoft, muhasebeizleme (preset) |
| `provider/izibiz` | SOAP | İzibiz |
| `provider/edm` | SOAP | EDM Bilişim |
| `provider/nes` | REST | NES |
| `provider/nilvera` | REST | Nilvera |
| `provider/turkcell` | REST | Turkcell e-Şirket |
| `provider/qnb` | SOAP | QNB eSolutions (eFinans) |
| `provider/sovos` | SOAP | Sovos Türkiye (FIT / Foriba) |
| `provider/trendyol` | REST | Trendyol E-Faturam |

## XML output

`encoding/xml` writes namespace URIs on elements. Prefixes such as `cbc:` / `cac:` are not guaranteed. That is valid XML; Schematron XPath binds prefixes to URIs, so prefix spelling is not required for correctness. This library does not rewrite serialized XML with string replace or regex.

Extra `xmlns` declarations that are not part of the generated structs (for example the `n4` wildcard in GİB sample `ExtensionContent`) are not round-tripped. Official sample XML still unmarshals and still XSD-validates as original bytes.

## Schemas and license

Vendored artefacts live in `schemas/ubltr-1.2.1/` and `schemas/earchive/` (also embedded for validators):

- XSD: [UBL-TR 1.2.1 Paketi](https://ebelge.gib.gov.tr/dosyalar/kilavuzlar/UBL-TR1.2.1_Paketi.zip)
- Schematron and envelope: [e-Fatura Paketi, 24 August 2026](https://ebelge.gib.gov.tr/dosyalar/kilavuzlar/e-FaturaPaketi%20(29).zip)
- e-Arşiv report: [GİB `earsiv_paket_v1.20`](https://ebelge.gib.gov.tr/dosyalar/kilavuzlar/earsiv_paket_v1.20.zip) (`EArsiv.xsd`, 5 November 2026)

The Go code is MIT (`LICENSE`). OASIS UBL 2.1 and UN/CEFACT notices stay in the schema files. GİB publishes no OSS license; files are vendored unmodified. See `NOTICE`.

GİB may update packages independently of this module’s version. Check [ebelge.gib.gov.tr](https://ebelge.gib.gov.tr/).

## Development

```bash
make generate   # regenerate XSD types and GİB code lists
make test       # all workspace modules (PDF tests skip if Chrome is missing)
make check      # format check, go vet, golangci-lint, then tests
```

This repository is a Go workspace (`go.work`). `GOWORK=off go test ./...` tests only the core module.

Layout:

- `schemas/` — official GİB/OASIS XSD and Schematron files (source of truth)
- `schema/` — generated Go types (`schema/ubltr121/...`, `schema/earchive`, `schema/envelope`); do not edit by hand
- `codelist/` — generated GİB unit/currency/profile/tax/… constants
- `builder/` — fluent builders on top of generated types
- `render/` — HTML/XSLT module
- `render/chromium/` — PDF adapter
- `examples/render/` — HTML, custom XSLT, and PDF programs
- `cmd/ubltr/` — nested `ubltr` CLI module
- `provider/` — integrator contract; `provider/<name>/` — one module per integrator; `provider/watch/` — polling to events
- `docs/` — install, usage, builder, rendering, CLI, testing, and repository-settings guides

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). Please follow the [code of conduct](CODE_OF_CONDUCT.md). Security reports go through [SECURITY.md](SECURITY.md), not public issues.

## License

The Go code is MIT. See [LICENSE](LICENSE) and [NOTICE](NOTICE) for OASIS, UN/CEFACT, and GİB artefact terms.
