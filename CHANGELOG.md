# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- XSD-driven Invoice types for UBL-TR 1.2.1 (`schema/ubltr121`) and `ubltr-gen`.
- `Marshal`, `Unmarshal`, `NewInvoice`, and `ValidateXSD` against the GİB Invoice schema.
- `ValidateSchematron`, `ValidateSchematronType`, and `Validate` (XSD then Schematron) using flattened GİB ISO rules and XPath 3.1.
- Vendored GİB UBL-TR 1.2.1 XSD, Schematron, and envelope files under `schemas/`.

### Known limitations

- Official 2009 GİB sample invoices are XSD-valid and fail current Schematron (TCKN length and similar rules).
- CreditNote and other UBL-TR document types are not generated.
- `encoding/xml` does not guarantee `cbc:` / `cac:` prefixes.

[Unreleased]: https://github.com/furkanmeclis/go-ubltr/compare/HEAD...HEAD
