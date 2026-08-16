# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- XSD-driven Invoice types for UBL-TR 1.2.1 (`schema/ubltr121`) and `ubltr-gen`.
- `Marshal`, `Unmarshal`, `NewInvoice`, and `ValidateXSD` against the GİB Invoice schema.
- `ValidateSchematron`, `ValidateSchematronType`, and `Validate` (XSD then Schematron) using flattened GİB ISO rules and XPath 3.1.
- Nested `render` module: XML → XSLT → HTML via helium xslt3, with a user-supplied or `StyleMinimal` stylesheet.
- Nested `render/chromium` module: HTML → PDF via chromedp (A4). Chrome is a runtime dependency, not a core `go get` dependency.
- How-to docs under `docs/` (install, usage, builder, rendering, testing).
- Vendored GİB UBL-TR 1.2.1 XSD, Schematron, and envelope files under `schemas/`.
- Fluent Invoice builders in package `builder` (party/line/tax/payment/address/delivery) on top of generated types.
- `builder.Money` / `Quantity` decimal helpers (`shopspring/decimal`, not `float64`) with automatic line and document totals.
- Generated GİB code lists in package `codelist` from `UBL-TR_Codelist.xml` (full unit, currency, profile, tax, MIME, … plus `Valid*` helpers).
- Structured `Violation` fields (`Path`, `RuleID`, `Severity`) on XSD and Schematron errors.

### Known limitations

- Official 2009 GİB sample invoices are XSD-valid and fail current Schematron (TCKN length and similar rules).
- GİB viewer XSLT is not bundled; supply your own stylesheet.
- CreditNote and other UBL-TR document types are not generated.
- `encoding/xml` does not guarantee `cbc:` / `cac:` prefixes.

[Unreleased]: https://github.com/furkanmeclis/go-ubltr/compare/HEAD...HEAD
