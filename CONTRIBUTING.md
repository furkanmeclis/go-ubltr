# Contributing

Thank you for contributing to go-ubltr.

## Prerequisites

- Go 1.26.1 or later (see `go.mod`)
- Make
- [golangci-lint](https://golangci-lint.run/) v2 (for `make lint` / `make check`)

Chrome or Chromium is optional. PDF tests skip when it is not on `PATH`.

## Quick start

```bash
git clone https://github.com/furkanmeclis/go-ubltr.git
cd go-ubltr
make test
make check
```

`make test` runs the core module, `render`, `render/chromium`, and `cmd/ubltr`. PDF tests skip if Chrome is missing.

## Development workflow

1. Fork and create a branch.
2. Make a focused change. Do not mix unrelated refactors.
3. Add or update tests for behaviour you change.
4. Run `make check`.
5. Open a pull request using the template.

## Project rules

- **XSD is the source of truth.** Do not invent structs from sample XML.
- **Do not edit generated files** under `schema/`, `codelist/generated.go`, or `codelist/lookup.go`. Change the generator or vendored schema, then `make generate`.
- **Keep the core module free of chromedp.** HTML lives in `render`; PDF lives in `render/chromium`. Confirm with `GOWORK=off go list -m all | grep chromedp` (must be empty).
- **Do not use `float64` for money.** Use `builder.Money` / `Quantity`.
- **Error strings** are lowercase and have no trailing punctuation.
- Keep official names (`GİB`, `UBL-TR`, `e-Fatura`, `e-Arşiv`) when they are the correct terms. Do not translate vendored schemas or sample XML.

## Privacy

Never commit, paste, or attach real invoices, taxpayer identifiers (TCKN / VKN), credentials, customer data, or live GİB payloads. Use the fixtures under `testdata/` or synthetic documents.

## Documentation

Update `CHANGELOG.md` `[Unreleased]` for user-visible changes. How-to pages live in [`docs/`](docs/README.md). Repository About fields are documented in [`docs/repository-settings.md`](docs/repository-settings.md).

## Reporting issues

Use [GitHub Issues](https://github.com/furkanmeclis/go-ubltr/issues) with the bug or feature form. Include Go version, OS, and a **synthetic** XML snippet when it helps. Security reports: [SECURITY.md](SECURITY.md).
