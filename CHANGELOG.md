# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- XSD-driven Invoice, CreditNote, ApplicationResponse, DespatchAdvice, and ReceiptAdvice types for UBL-TR 1.2.1 (`schema/ubltr121`) and `ubltr-gen`.
- `Marshal`, `Unmarshal`, `NewInvoice`, and `ValidateXSD` (schema chosen from the document root).
- `ValidateSchematron`, `ValidateSchematronType`, and `Validate` (XSD then Invoice Schematron) using flattened GİB ISO rules and XPath 3.1.
- Nested `render` module: XML → XSLT → HTML via helium xslt3, with a user-supplied or `StyleMinimal` stylesheet.
- Nested `render/chromium` module: HTML → PDF via chromedp (A4). Chrome is a runtime dependency, not a core `go get` dependency.
- How-to docs under `docs/` (install, usage, builder, rendering, CLI, testing).
- Vendored GİB UBL-TR 1.2.1 XSD, Schematron, and envelope files under `schemas/`.
- Fluent builders in package `builder` (invoice, credit note, application response, despatch, receipt, plus party/line/tax/payment/address/delivery) on top of generated types.
- `builder.Money` / `Quantity` decimal helpers (`shopspring/decimal`, not `float64`) with automatic line and document totals.
- Generated GİB code lists in package `codelist` from `UBL-TR_Codelist.xml` (full unit, currency, profile, tax, MIME, … plus `Valid*` helpers).
- `ubltr.Detect` (Invoice, CreditNote, ApplicationResponse, DespatchAdvice, ReceiptAdvice, e-Arşiv report) and `ValidateXSD` dispatch on the XML root.
- `NewCreditNote`, `NewApplicationResponse`, `NewDespatchAdvice`, `NewReceiptAdvice`, `NewEArchiveReport`, and `NewCreditNoteFromInvoice`.
- Generated types and builders for CreditNote, ApplicationResponse, DespatchAdvice, and ReceiptAdvice (`schema/ubltr121/…`, shared `cac` / `cbc`).
- Generated e-Arşiv *report* types from GİB `EArsiv.xsd` v1.1.8 (`schema/earchive`, root `eArsivRaporu`). Distinct from e-Arşiv invoices (UBL Invoice + `ProfileID` `EARSIVFATURA`). `faturaTip` includes `YTBTEVKIFAT` and `YTBTEVKIFATIADE`. `ds:Signature` is InnerXML (no XAdES Go model).
- `CloneInvoice`, `CloneCreditNote`, `Inspect`, `Diff`, `AddAttachment` / `AddAdditionalDocumentReference`, and `UnmarshalReader` / `MarshalWrite`.
- Nested `ubltr` CLI (`cmd/ubltr`): `validate`, `inspect`, `render`, `generate`.
- Repository CI, golangci-lint, contribution guide, security policy, and code of conduct.
- Withholding tax (tevkifat): `TaxBuilder.Withholding` / `WithholdingName`, line and document `WithholdingTaxTotal`, `PayableAmount` = tax-inclusive − withheld, and type checks for TEVKIFAT and related types.
- Return invoices: `InvoiceBuilder.BillingReference`, `BillingReferenceCode`, `ReturnOf`, and `builder.NewReturnInvoiceFrom` with `ReturnType`, `ReturnLines`, `ReturnSameParties` (DocumentTypeCode `IADE`, swapped parties, TEMELFATURA fallback).
- `InvoiceBuilder.ExchangeRate` (`cac:PricingExchangeRate`, required for non-TRY invoices), `OrderReference`, `DespatchReference`, `AdditionalDocument`, and `AdditionalDocumentReference`.
- IHRACAT support: `BuyerCustomer` / `BuyerCustomerBuilder`, `TaxRepresentative` / `TaxRepresentativeBuilder`, `NewExportCustomerBuilder`, `NewExportBuyerBuilder`, `NewTaxFreeBuyerBuilder`, `PartyBuilder.LegalEntity` / `Nationality`, line-level `InvoiceLineBuilder.Delivery` / `Note`, and `DeliveryBuilder.Incoterms`, `SpecialTerms`, `ShipmentID`, `CustomsID` (GTİP), `TransportMode`, `Package`.
- Exemption codes are checked in `TaxBuilder.Exemption` and kept on the document `TaxTotal`.
- `builder.AmountInWords` (Turkish amount in words) and `InvoiceBuilder.AmountInWordsNote`.
- Embedded XSLT: `InvoiceBuilder.XSLT` and `builder.XSLTReference` (`AdditionalDocumentReference` with DocumentType `XSLT`).
- `codelist` aliases `ProfileIhracat`, `InvoiceTypeTevkifat`, `InvoiceTypeTevkifatIade`, `InvoiceTypeIstisna`.
- Golden builder invoices under `testdata/valid/`, checked with XSD and GİB Schematron (`-update` regenerates them).

### Known limitations

- Official 2009 GİB sample invoices are XSD-valid and fail current Schematron (TCKN length and similar rules).
- GİB viewer XSLT is not bundled; supply your own stylesheet.
- GİB envelope types are not generated.
- `encoding/xml` does not guarantee `cbc:` / `cac:` prefixes.
- The vendored GİB Schematron rejects `WithholdingTaxTotal` on TEVKIFATIADE and YTBTEVKIFATIADE; `NewReturnInvoiceFrom` uses IADE / YTBIADE for withholding returns.

[Unreleased]: https://github.com/furkanmeclis/go-ubltr/compare/HEAD...HEAD
