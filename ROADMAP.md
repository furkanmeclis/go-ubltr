# Roadmap

go-ubltr already supports:

- UBL-TR 1.2.1 Invoice, CreditNote, ApplicationResponse, DespatchAdvice, and ReceiptAdvice types generated from XSD
- e-Arşiv *report* types from GİB `EArsiv.xsd` (root `eArsivRaporu`; distinct from e-Arşiv invoices)
- Marshal / unmarshal, XSD validation by document kind, and GİB Schematron for Invoice, ApplicationResponse, DespatchAdvice (e-İrsaliye), ReceiptAdvice, and the envelope (`efatura` / `earchive` / `goruntuleme`)
- Structured validation errors: rule ID, pattern ID, XPath-like location, line, and severity
- GİB envelope types generated from the vendored envelope XSDs, plus helpers to build, validate, and unpack envelopes
- Fluent builders, decimal money/quantity, and generated GİB code lists
- Clone, inspect, attachments, XML diff, and reader/writer helpers
- Nested HTML (`render`) and PDF (`render/chromium`) modules
- Nested `ubltr` CLI (`validate`, `inspect`, `render`, `generate`, `watch`) with `--format=json` for `validate` and `inspect`
- Vendor-neutral özel entegratör contract in the nested `provider` module (capabilities, normalized statuses and errors, GİB envelope codes)
- Integrator modules: Uyumsoft (and the muhasebeizleme preset), İzibiz, EDM, NES, Nilvera, Turkcell e-Şirket, QNB eSolutions, Sovos, Trendyol E-Faturam
- `provider/watch`: polling integrators into events, with file, Redis and SQL state

Known limitations are listed in [CHANGELOG.md](CHANGELOG.md).

## Possible later work

These items are optional.

1. **More integrators** — modules for integrators whose API documents are given on request (eLogo, Hızlı Bilişim, Mysoft, Veriban, Edoksis, İşNet …) once their documentation and test accounts are available.
2. **Schema refresh** — vendor newer GİB XSD, Schematron, envelope, and code-list packages when they are published, and regenerate types. Record checksums in `NOTICE`.

## Out of scope

Not planned for this repository:

| Topic | Reason |
| --- | --- |
| Integrator clients in the core module | They live in nested `provider/*` modules so core stays XML-only |
| Direct GİB web-service (doğrudan entegrasyon) | Needs a mali mühür and GİB accreditation; integrators cover it |
| XML signature / XAdES signing | Key handling; `ds:Signature` stays InnerXML |
| Bundling GİB viewer XSLT in core | Callers supply a stylesheet; `examples/render/general` is a local trial |
| Rewriting `cbc:` / `cac:` prefixes | Valid XML does not require those prefixes |
| Chrome / chromedp in the core module | PDF stays in `render/chromium` |
| ERP or accounting adapters | Those systems should depend on this library, not the reverse |
