# e-Arşiv invoice vs e-Arşiv report

**e-Arşiv invoice ≠ e-Arşiv report.**

They are different XML documents, different schemas, and different recipients. go-ubltr does not treat `EArsiv.xsd` as the e-Arşiv invoice schema.

This page is a technical integration note. It is not legal advice. Check current GİB legislation, the current GİB technical guides, and the technical documentation of any private integrator (özel entegratör) you use.

## Two documents

```
e-Arşiv invoice  →  customer   (the commercial invoice)
e-Arşiv report   →  GİB        (period reporting of those documents)
```

| Document | Schema | Purpose |
| --- | --- | --- |
| UBL-TR Invoice | `UBL-Invoice-2.1.xsd` plus UBL-TR rules | The actual e-Fatura / e-Arşiv invoice |
| e-Arşiv report | `EArsiv.xsd` | Reporting e-Arşiv documents to GİB |
| Envelope | Package / SBDH XSD | Transport wrapper around documents |

## e-Arşiv invoice

The e-Arşiv invoice is the commercial invoice issued to the customer.

It is a **UBL-TR Invoice**. The GİB profile is `EARSIVFATURA`:

```xml
<cbc:ProfileID>EARSIVFATURA</cbc:ProfileID>
```

In go-ubltr:

```
Invoice
    ↓
UBL-TR XML
    ↓
ProfileID = EARSIVFATURA
```

That XML is the document presented to the customer. It is not built from `EArsiv.xsd`.

Construction, XSD, Schematron, and rendering follow the Invoice path:

- `builder.NewInvoiceBuilder().Profile(codelist.ProfileEARSIVFATURA)` (or `codelist.ProfileEArsivFatura`)
- `ubltr.ValidateXSD` against `UBL-Invoice-2.1.xsd`
- `ubltr.ValidateSchematronType(ctx, xml, ubltr.TypeEArchive)` for Invoice business rules with `type=earchive`

`ubltr.TypeEArchive` (`"earchive"`) is a **Schematron parameter for Invoice**. It is not the e-Arşiv report document kind.

## e-Arşiv report

The e-Arşiv report is **not** the invoice.

It is a separate XML format used to report e-Arşiv documents issued in a period to GİB. GİB defines it in `EArsiv.xsd` (root `eArsivRaporu`, namespace `http://earsiv.efatura.gov.tr`). A report may list invoices, cancellations, objections, and other e-Arşiv document rows for that period.

```
e-Arşiv invoice  →  customer
e-Arşiv report   →  GİB
```

`ubltr.Detect` returns `KindEArchiveReport` for `eArsivRaporu`. That kind is not an Invoice and not `ProfileID`.

## Do not mix the schemas

Wrong:

> “Use `EArsiv.xsd` to create an e-Arşiv invoice.”

Right:

> An e-Arşiv invoice is a UBL-TR Invoice with `ProfileID = EARSIVFATURA`.
>
> `EArsiv.xsd` defines the e-Arşiv **report** format.

go-ubltr’s core package does not assume `EArsiv.xsd` means “e-Arşiv invoice”.

## Private integrators (özel entegratör)

Most go-ubltr users are expected to send documents through a private integrator, not to GİB’s own web services.

In that design, issuing an e-Arşiv invoice is:

```
Invoice XML
    ↓
ProfileID = EARSIVFATURA
    ↓
Provider / integrator
```

The application normally **does not** also build an e-Arşiv report and submit it to GİB. The integrator’s contract and technical API decide how period reporting is handled.

Example:

```
Rent-a-car app
    ↓
Build Invoice
    ↓
UBL-TR Invoice
    ↓
ProfileID = EARSIVFATURA
    ↓
go-ubltr validation
    ↓
Integrator
    ↓
GİB
```

How the integrator treats the e-Arşiv report (whether it builds `eArsivRaporu`, when it sends it, which API) varies by provider. Do not copy another provider’s report flow.

Users who integrate with GİB **directly** from their own IT system are a different case. Their e-Arşiv reporting duties are not the same as the integrator-client flow above. Evaluate those requirements against current GİB rules and the role your system actually plays.

## What go-ubltr covers

**e-Arşiv invoice (UBL-TR Invoice):**

- Produce UBL-TR Invoice XML, including `ProfileID=EARSIVFATURA`
- XSD validation (`UBL-Invoice-2.1.xsd`)
- Schematron validation (`type=earchive`)
- HTML/PDF rendering of that Invoice XML (nested `render` modules)

**e-Arşiv report (`EArsiv.xsd`):**

- The report schema is vendored under [`schemas/earchive/`](../schemas/earchive/) (`EArsiv.xsd` from GİB `earsiv_paket_v1.20`)
- Generated types live in [`schema/earchive`](../schema/earchive/) (`eArsivRaporu`, not Invoice)
- `Detect`, `ValidateXSD`, `NewEArchiveReport`, and `builder.NewEArchiveReportBuilder` work on that report XML

Having report types in the library does not mean a typical integrator client should produce or submit reports. Treat report XML as a separate concern from issuing the customer invoice. Direct GİB integrators should decide separately whether they need it.

Related how-tos: [usage](usage.md), [builder](builder.md), [rendering](rendering.md).

## Official sources

GİB publishes e-Arşiv guides and packages on [ebelge.gib.gov.tr](https://ebelge.gib.gov.tr/). Packages and guide numbers change independently of this module. Always use the copies currently listed there.

This repository vendors the e-Arşiv **report** schema from GİB package `earsiv_paket_v1.20` (see `NOTICE`). Invoice rules still come from UBL-TR 1.2.1 and the e-Fatura Schematron package.

The e-Arşiv Teknik Kılavuzu is listed by GİB as:

[https://ebelge.gib.gov.tr/dosyalar/kilavuzlar/e-Arsiv_Teknik_Kilavuzu_V.1.18.pdf](https://ebelge.gib.gov.tr/dosyalar/kilavuzlar/e-Arsiv_Teknik_Kilavuzu_V.1.18.pdf)

That filename is **V.1.18**. Do not treat the version number as frozen; confirm the current file on the GİB site before relying on a specific edition.
