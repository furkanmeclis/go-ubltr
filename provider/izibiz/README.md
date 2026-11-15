# provider/izibiz

İzibiz özel entegratör client for the [`provider`](../README.md) contract.
SOAP only: AuthenticationWS, EInvoiceWS, EIArchiveWS and EIrsaliyeWS.

```bash
go get github.com/furkanmeclis/go-ubltr/provider/izibiz
```

## Configuration

```go
import (
	"github.com/furkanmeclis/go-ubltr/provider"
	"github.com/furkanmeclis/go-ubltr/provider/izibiz"
)

p, err := izibiz.New(izibiz.Config{
	Username: os.Getenv("IZIBIZ_USERNAME"),
	Password: os.Getenv("IZIBIZ_PASSWORD"),
	Test:     true, // https://efaturatest.izibiz.com.tr
})

// or through the registry
p, err := provider.Open("izibiz", provider.Config{Username: u, Password: pw, Test: true})
```

| `Config` field | Registry option | Sent as |
| --- | --- | --- |
| `Endpoint` | `Config.Endpoint` | Base URL. Required outside the test system: İzibiz does not publish the production host. |
| `ApplicationName` | `application_name` | `REQUEST_HEADER/APPLICATION_NAME` (default `go-ubltr`) |
| `ChannelName` | `channel_name` | `REQUEST_HEADER/CHANNEL_NAME` |
| `InvoiceSeriesPrefix` | `invoice_series_prefix` | `SERI_PREFIX`; İzibiz assigns the e-Fatura number |
| `ArchiveSeries` | `archive_series` | `EARSIV_PROPERTIES/SERI`; İzibiz assigns the e-Arşiv number |
| `EArchiveType` | `earchive_type` | `EARSIV_TYPE`: `NORMAL` (default) or `INTERNET` |
| `XSLTName` | `xslt_name` | `XSLT_NAME` (portal template, e.g. `DEFAULT`) |

Services are reached below the base URL at `/AuthenticationWS`,
`/EInvoiceWS`, `/EIArchiveWS/EFaturaArchive` and `/EIrsaliyeWS/EIrsaliye`.

### Session

`Login` runs on the first call. The `SESSION_ID` is shared by all
goroutines (İzibiz keeps it for 8 hours). When a call fails with a session
error (ERROR_CODE `10004`, `2005`, or a message about the session), the
module logs in once more and repeats the call. Call `Logout` when done.

## Capabilities

| Kind | Operations | İzibiz method |
| --- | --- | --- |
| `efatura` | send | EInvoiceWS `SendInvoice` (zipped, `COMPRESSED=Y`) |
| | status | `GetInvoiceStatus` |
| | list-inbox / list-outbox | `GetInvoice` (`HEADER_ONLY=Y`, `DIRECTION` IN/OUT) |
| | fetch (ubl, pdf, html) | `GetInvoice` / `GetInvoiceWithType` |
| | respond | `SendInvoiceResponseWithServerSign` (`KABUL` / `RED`) |
| | mark-taken | `MarkInvoice` (`READ`) |
| | check-user | AuthenticationWS `CheckUser` (`DOCUMENT_TYPE=INVOICE`) |
| `earsiv` | send | EIArchiveWS `WriteToArchiveExtended` |
| | status | `GetEArchiveInvoiceStatus` |
| | list-outbox | `GetEArchiveInvoiceList` (`HEADER_ONLY=Y`, `READ_INCLUDED=Y`, date windows) |
| | fetch (ubl, pdf, html) | `ReadFromArchive` |
| | cancel-earchive | `CancelEArchiveInvoice` (reason → `IPTAL_NOTU`) |
| `eirsaliye` | send | EIrsaliyeWS `SendDespatchAdvice` |
| | status | `GetDespatchAdviceStatus` |

Other flags: `SignsDocuments`, `AcceptsRawUBL`, `MaxBatch` 100 (the
documented per-request limit), `MinPollInterval` 15 minutes (the documented
minimum for inbox polling; status queries should be at least 4 hours
apart), `AssignsNumbers` when a series is configured.

Listings have no cursor. Use `OnlyNew: true` and `MarkTaken` to work
through new documents, as İzibiz recommends. Date filters use
`DATE_TYPE=CREATE`.

### e-Arşiv outbox listing

`ListOutbox` with `Kind: provider.EArchive` calls EIArchiveWS
`GetEArchiveInvoiceList`. The request sends `LIMIT`, `START_DATE`,
`END_DATE`, `HEADER_ONLY=Y` and `READ_INCLUDED=Y`. It leaves out `ID`,
`UUID`, `PERIOD`, `PREFIX`, `REPORT_*` and `CONTENT_TYPE`. The schema has
no cursor, so the module pages by date window:

- Without `From`, one call is made and `Next` is empty. `To` alone is
  sent as `END_DATE`.
- With `From` (`To` defaults to now), each call covers one 24-hour window.
  `Next` is the start of the following window (RFC 3339). Pass it back as
  `Cursor` with the same `From` and `To`. A window can be empty while
  `Next` is still set.
- Both window bounds are sent, so a document exactly on a boundary can
  appear on two pages. De-duplicate by UUID.
- `Limit` applies per window and is capped at 10000, the XSD's
  `maxOccurs` for `INVOICE`.
- Dates are written as `YYYY-MM-DDTHH:mm:ss` in Turkey time (UTC+3),
  as in the vendor sample. The schema does not say whether the filter
  applies to the issue date or the creation date.
- `OnlyNew` is rejected: `MarkTaken` marks e-Fatura documents only.
- An `ERROR_CODE` 10008 (no matching record) becomes an empty page.

Items map `HEADER/INVOICE_ID`, `UUID`, `PROFILE_ID`, `INVOICE_TYPE`,
`ISSUE_DATE`, `SENDER_*`, `CUSTOMER_*` (receiver), `PAYABLE_AMOUNT` and
`CURRENCY_CODE`. The status comes from `STATUS_CODE`, or from `STATUS`
when there is no code, using the e-Arşiv table below. The list header
has no GİB status code or creation time, so `GIBCode` is 0 and
`CreatedAt` is zero.

This operation has not been checked against the live İzibiz system. The
fixture is built from the XSD, and the status values in a listing are
assumed to match `GetEArchiveInvoiceStatus`.

Schema notes:

- The XSD declares the element `"REPORT_ID "` with a trailing space,
  which is not a valid name. Integration notes that list the fields
  leave it out. The module never sends it.
- `READ_INCLUDED` is `xsd:string` here, not boolean as in EInvoiceWS.
  The module sends `Y`, as the vendor test code does.

### Status mapping

`STATUS_CODE` (100–141) and the `STATUS - SUBSTATUS` text map to
`provider.Status` using the Developer Portal tables, for example 111
(ALICIDAN ONAY BEKLİYOR) → `delivered`, 112 → `accepted`, 120 → `rejected`,
136 → `failed`, 133 → `received`. A failing `GIB_STATUS_CODE` overrides an
in-progress vendor status. e-Arşiv: 100 `queued`, 105 `draft`, 110
`processing`, 120 `sent` (to be reported), 130 `delivered` (reported),
150 `cancelled`, 200 `failed`.

## Errors

[`errors.csv`](errors.csv) lists the `ERROR_CODE` values published on the
Developer Portal (-1, 10001–10018) plus 2005, which the vendor sample
client treats as an expired session. `ERROR_TYPE` in a response body and
`RequestFault` details in SOAP faults are both read. Codes keep their
original value in `Error.Code`; GİB codes go to `Error.GIBCode`.

## Sources

- WSDL/XSD from `https://efaturatest.izibiz.com.tr` (copies in
  [`testdata/wsdl`](testdata/wsdl)).
- İzibiz Developer Portal, `https://dev.izibiz.com.tr`.
- Sample client `github.com/izibiz/ws-client-dotnet`.

Request bodies produced by the tests validate against the stored XSDs.

## Live tests

Live tests only read (CheckUser, a header-only outbox listing, an optional
status query) and use your own account:

```bash
IZIBIZ_USERNAME=... IZIBIZ_PASSWORD=... go test -tags live -run Live ./...
# optional: IZIBIZ_ENDPOINT=https://... IZIBIZ_UUID=<ETTN>
```

## Not covered

- Production base URL (pass `Endpoint`).
- Inbound e-Arşiv, e-İrsaliye listing, fetch and receipt advice
  (`GetDespatchAdvice`, `SendReceiptAdvice`), credits.
- MTOM attachments: responses are read from the XML part only, which is
  enough for the operations above.
