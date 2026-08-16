# Roadmap

go-ubltr already supports:

- UBL-TR 1.2.1 Invoice, CreditNote, ApplicationResponse, DespatchAdvice, and ReceiptAdvice types generated from XSD
- e-Arşiv *report* types from GİB `EArsiv.xsd` (root `eArsivRaporu`; distinct from e-Arşiv invoices)
- Marshal / unmarshal, XSD validation by document kind, and Invoice Schematron (`efatura` / `earchive`)
- Fluent builders, decimal money/quantity, and generated GİB code lists
- Clone, inspect, attachments, XML diff, and reader/writer helpers
- Nested HTML (`render`) and PDF (`render/chromium`) modules
- Nested `ubltr` CLI (`validate`, `inspect`, `render`, `generate`)

Known limitations are listed in [CHANGELOG.md](CHANGELOG.md).

## Possible later work

These items are optional. They will only land if they stay small and document-centric.

1. **Schema refresh** — vendor newer GİB XSD, Schematron, envelope, and code-list packages when they are published, and regenerate types. Record checksums in `NOTICE`.
2. **Envelope types** — generate Go types from the already-vendored GİB envelope XSDs under `schemas/ubltr-1.2.1/envelope/`.
3. **Richer validation errors** — expose path, rule ID, and severity that already exist internally, without changing the validation engines.
4. **CLI machine-readable output** — optional `--format=json` for `ubltr inspect` / `ubltr validate`. Human text remains the default.

## Out of scope

Not planned for this repository:

| Topic | Reason |
| --- | --- |
| GİB web-service or özel entegratör HTTP clients | Transport and vendor APIs, not UBL-TR XML |
| XML signature / XAdES signing | Key handling; `ds:Signature` stays InnerXML |
| Bundling GİB viewer XSLT in core | Callers supply a stylesheet; `examples/render/general` is a local trial |
| Rewriting `cbc:` / `cac:` prefixes | Valid XML does not require those prefixes |
| Chrome / chromedp in the core module | PDF stays in `render/chromium` |
| ERP or accounting adapters | Those systems should depend on this library, not the reverse |
