# Testing

## Default (this repo)

```bash
make test
```

That runs:

```bash
go test ./...
go test ./render/...
go test ./render/chromium/...
```

`go test ./...` from the repo root only covers the **core** module. Nested modules have their own `go.mod`, so they are listed separately in the Makefile.

PDF tests need Chrome or Chromium. If the binary is missing they **skip**, they do not fail.

## Core only (no render, no Chrome)

```bash
GOWORK=off go test ./...
```

`GOWORK=off` ignores `go.work` and behaves like a consumer who only imported `github.com/furkanmeclis/go-ubltr`. Use this to confirm chromedp is not on the core module graph:

```bash
GOWORK=off go list -m all | grep chromedp   # empty
```

## What the tests cover

| Area | Package | Notes |
| --- | --- | --- |
| Unmarshal / marshal | `ubltr_test` | `testdata/ubltr-1.2.1/TemelFaturaOrnegi.xml` |
| XSD | `ubltr_test`, `internal/validation` | fixture passes; empty Invoice fails |
| Schematron | `ubltr_test`, `internal/validation` | fixture is expected to fail current rules (TCKN); unknown ProfileID fails |
| HTML | `render` | `StyleMinimal` on the GİB sample; Turkish glyphs; malformed XML |
| Builder | `builder` | fluent Invoice → XSD-valid XML; totals rounding fixture |
| Code lists | `codelist` | every GİB unit/currency/profile code from `UBL-TR_Codelist.xml` |
| XSLT host access | `render/xslt/helium` | `document('/etc/passwd')` must not read the disk |
| PDF pipeline (fake) | `render` | stub `PDFRenderer` |
| PDF smoke | `render/chromium` | real Chrome; `%PDF` prefix and a page object |

Table-driven tests use named `t.Run` cases. Independent tests call `t.Parallel()` except the Chrome smoke test.

## Fixtures

- `testdata/ubltr-1.2.1/*.xml` — official GİB samples (XSD-valid, Schematron-invalid today).
- `codelist/` — generated from `schemas/ubltr-1.2.1/schematron/UBL-TR_Codelist.xml`.
- `render/testdata/styles/minimal.xsl` — MIT test stylesheet.
- `examples/render/general/general.xslt` — GİB viewer sheet for manual runs, not a unit-test input in core.

## Regenerating types

```bash
make generate
go test ./...
```

If generated files under `schema/` or `codelist/` change, include them in the same commit as the generator or schema update.

## Examples

Examples are `main` packages without `_test.go`. Run them with `go run .` from each directory; see [rendering.md](rendering.md) and [builder.md](builder.md).
