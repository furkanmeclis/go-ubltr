# go-ubltr

Go library for GİB UBL-TR e-Fatura / e-Arşiv Invoice XML.

**UBL-TR schema definitions are the source of truth. Generated Go types are derived from the schemas.**

Do not edit files under `schema/`. Regenerated types come from the vendored XSD in `schemas/` via `ubltr-gen`.

## Install

Requires Go 1.26.1 or later.

```bash
go get github.com/furkanmeclis/go-ubltr
```

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

	out, err := ubltr.Marshal(&inv)
	if err != nil {
		panic(err)
	}
	_ = out
}
```

`ubltr.NewInvoice()` returns an empty `*invoice.Invoice`. Builders for Party, Tax, and Line are not in this release.

## What is generated

`go generate ./schema/ubltr121` runs `cmd/ubltr-gen` against the GİB Invoice XSD:

```
schemas/ubltr-1.2.1/xsdrt/maindoc/UBL-Invoice-2.1.xsd
        ↓
schema/ubltr121/{invoice,cac,cbc,ext,udt}
```

Only types reachable from the Invoice element are emitted. CreditNote, DespatchAdvice, ReceiptAdvice, ApplicationResponse, and the GİB envelope are not generated yet.

Amounts, dates, and times are `string` so lexical values survive round-trip. Identifier types keep `schemeID`.

## Validation

| Layer | Status |
| --- | --- |
| XSD (Invoice) | Supported via `ValidateXSD`. Uses [helium](https://github.com/lestrrat-go/helium) against the vendored GİB UBL-TR 1.2.1 Invoice schema. |
| Schematron | Not supported yet. |

GİB `UBL-TR_Main_Schematron.xml` is ISO Schematron with `sch:include`, schema-level `let`, abstract rules, and `sch:extends`. Helium’s native compiler does not accept those constructs (it expects a flattened pattern/assert document). An ISO preprocess → SVRL pipeline is not wired. Do not treat XSD success as GİB business-rule success.

e-Arşiv invoices use the same Invoice XSD with `ProfileID=EARSIVFATURA`. Schematron `type` (`efatura` vs `earchive`) is not applied until Schematron runs.

## XML output

`encoding/xml` writes namespace URIs on elements. Prefixes such as `cbc:` / `cac:` are not guaranteed. That is valid XML; Schematron XPath binds prefixes to URIs, so prefix spelling is not required for correctness. This library does not rewrite serialized XML with string replace or regex.

Extra `xmlns` declarations that are not part of the generated structs (for example the `n4` wildcard in GİB sample `ExtensionContent`) are not round-tripped. Official sample XML still unmarshals and still XSD-validates as original bytes.

## Schemas and license

Vendored artefacts live in `schemas/ubltr-1.2.1/` (also embedded for validators):

- XSD: [UBL-TR 1.2.1 Paketi](https://ebelge.gib.gov.tr/dosyalar/kilavuzlar/UBL-TR1.2.1_Paketi.zip)
- Schematron and envelope: [e-Fatura Paketi](https://ebelge.gib.gov.tr/dosyalar/kilavuzlar/e-FaturaPaketi.zip)

The Go code is MIT (`LICENSE`). OASIS UBL 2.1 and UN/CEFACT notices stay in the schema files. GİB publishes no OSS license; files are vendored unmodified. See `NOTICE`.

GİB may update packages independently of this module’s version. Check [ebelge.gib.gov.tr](https://ebelge.gib.gov.tr/).

## Development

```bash
make generate   # regenerate types from XSD
make test
```

Layout:

- `schemas/` — official GİB/OASIS XSD and Schematron files (source of truth)
- `schema/` — generated Go types (`schema/ubltr121/...`); do not edit by hand
