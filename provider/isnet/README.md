# provider/isnet

İşNet (NetteFatura) client for the [`provider`](../README.md) contract:
e-Fatura and e-Arşiv through `InvoiceService` and the GİB user lookup
through `AddressBookService` (SOAP 1.1, WCF). The module is read-only: it
lists, downloads and queries documents and never changes their state.

> **Not verified against the live system.** No İşNet test or live account
> was available. Request shapes come from the İşNet interface documents (as
> summarised in the integration note cited below) and the WSDL served by
> the İşNet test system; response parsing is tested with fixtures built
> from the documented samples. Treat the first real calls as a
> verification step (see [Open questions](#open-questions)).

```bash
go get github.com/furkanmeclis/go-ubltr/provider/isnet
```

## Configuration

İşNet has no user name, password or token. Access is the caller's
**source IP**, which İşNet allowlists, plus the company VKN sent as
`CompanyTaxCode` in every request.

```go
c, err := isnet.New(isnet.Config{
	VKN:  "1234567890", // CompanyTaxCode
	Test: true,         // İşNet test system; default is live
})
```

Or by name with the registry:

```go
import _ "github.com/furkanmeclis/go-ubltr/provider/isnet"

p, err := provider.Open("isnet", provider.Config{VKN: vkn, Test: true})
```

| Field | Notes |
| --- | --- |
| `VKN` | Required. Sent as `CompanyTaxCode`. Because the IP may be authorised for several companies, take it from a verified connection record, never from user input. |
| `Test` | Test system instead of live (see endpoints below). |
| `Endpoint`, `AddressBookEndpoint` | Override the `InvoiceService` / `AddressBookService` URLs. The WSDL also lists an `http://` port; requests always go to the `https://` address. |
| `VendorNumbers` | Company units (`CompanyVendorNumber`, list them with `Vendors`). Sent as `IncludeCompanyVendorNumberList` in e-Fatura searches, next to `IncludeMainCompany=true`. |
| `IPFiltered` | Treat timeouts on a custom endpoint as IP-allowlist failures. Implied for the live host. |
| `HTTP` | Custom `provider.Doer`. |
| `Timeout` | Per-call limit (default 2 min; negative disables). |
| `DialTimeout` | TCP connect + TLS handshake limit of the built-in HTTP client (default 15 s). |
| `PageSize` | Default `RecordsPerPage` (default 20). |

| Service | Test | Live |
| --- | --- | --- |
| InvoiceService | `https://einvoiceservicetest.isnet.net.tr/InvoiceService/ServiceContract/InvoiceService.svc` | `https://einvoiceservice.isnet.net.tr/InvoiceService/ServiceContract/InvoiceService.svc` |
| AddressBookService | `https://einvoiceservicetest.isnet.net.tr/AddressBookService/ServiceContract/AddressBookService.svc` | `https://einvoiceservice.isnet.net.tr/AddressBookService/ServiceContract/AddressBookService.svc` |

Registry `Options`: `addressbook_endpoint`, `vendor_numbers` (comma
separated), `ip_filtered`, `timeout`, `dial_timeout` (Go durations),
`page_size`. `provider.Config.Endpoint` is the InvoiceService URL;
`Username`, `Password` and `APIKey` are ignored.

## IP allowlist

İşNet opens the live system only to IPs it has allowlisted, and only after
the test phase. From any other IP the live service **does not answer at
all**: no SOAP fault, no HTTP error, the TCP connection simply gets no
reply. The caller sees a connect, TLS or read timeout.

This module turns that into a distinct error. When a call to an
IP-filtered endpoint (the live host, or any endpoint with
`IPFiltered: true`) fails with a timeout before an HTTP response arrives,
it returns:

```go
&provider.Error{
	Kind:      provider.KindPermission,
	Code:      "ip-not-allowlisted", // isnet.CodeIPNotAllowed
	Retryable: false,
	Message:   "İşNet yanıt vermedi; çağrıyı yapan IP İşNet'te tanımlı olmayabilir …",
	Err:       /* wraps isnet.ErrIPNotAllowed and the original timeout */,
}
```

```go
if isnet.IsIPNotAllowed(err) { // or errors.Is(err, isnet.ErrIPNotAllowed)
	// ask İşNet to allowlist the egress IP for this VKN
}
```

`KindPermission` rather than `KindTimeout`: the documented cause is a
missing authorisation on İşNet's side, and retrying from the same IP
cannot succeed, so `provider.Retry` and pollers must not keep retrying it.
`errors.Is(err, context.DeadlineExceeded)` still holds, so code that only
looks for timeouts sees one. A single genuine network outage looks the
same; the integration note suggests treating it as an IP problem only when
every call fails while other hosts are reachable. On the test system (and
any endpoint not marked IP-filtered) timeouts stay `KindTimeout` and
retryable. Connection refused or reset is not classified as an IP problem.

## Capabilities

| Operation | e-Fatura | e-Arşiv | İşNet method |
| --- | --- | --- | --- |
| ListInbox | ✓ | – | `SearchAllInvoice` (`InvoiceDirection=Incoming`) |
| ListOutbox | ✓ | ✓ | `SearchAllInvoice` (`Outgoing`) / `SearchArchiveInvoice` |
| Fetch UBL | ✓ in and out | ✓ | `SearchAllInvoice` / `SearchArchiveInvoice` with `Ettn`, `IsXMLIncluded` |
| Fetch PDF | ✓ | ✓ | same, `IsPDFIncluded` → `InvoicePdf` |
| Fetch HTML | ✓ | – | same, `IsHtmlIncluded` → `InvoiceHtml` (e-Arşiv items have no HTML member) |
| Status | ✓ in and out | ✓ | search by `Ettn` |
| CheckUser | ✓ | – | `GetTaxPayer` (AddressBookService) |

Extra methods: `Vendors` (`GetCompanyVendor`), `DocumentViewerLink`
(`GetDocumentViewerLink`, returned as received) and
`ListEArchiveCancelled` (`SearchArchiveInvoice` with `IsCancelled=true`).

Not implemented, on purpose: every `Send*`, `Cancel*`, `ArchiveInvoice`,
`DeArchiveInvoice`, `UpdateInvoiceState`, `SendInvoiceReply` and
`DirectInvoice` call. They change document state in the company's İşNet
account (the integration rules forbid them for a reading client), so
`OpSend`, `OpRespond`, `OpMarkTaken` and `OpCancelEArchive` are not
declared. `ListQuery.OnlyNew` is refused: there is no read-only "taken"
flag.

Behaviour:

- Every request lists its members in alphabetical order (WCF ignores
  members out of order) and omits empty ones. Dates are sent as
  `YYYY-MM-DDTHH:mm:ss` in Türkiye time without a zone.
- Listings filter `From` / `To` on `MinInvoiceCreationDate` /
  `MaxInvoiceCreationDate`; `Cursor` is a page number (`PageNumber` starts at
  1) and `Page.Next` follows `TotalNumberOfPages`, or a full page when that
  is missing. `IsArchiveIncluded` is always true so invoices archived in the
  İşNet portal are included.
- e-Fatura listings do not request the UBL. e-Arşiv listings do, so ETTN,
  number and profile can be read from the UBL when the item lacks them.
- For incoming e-Fatura the `Receiver` entry of the list item is reported as
  the sender, which is how the community integration reads it (unconfirmed).
- `Fetch` without `Direction` searches the outgoing box, then the incoming
  box. A zipped `InvoiceXml` is unpacked.
- e-Arşiv `Status` runs a second search with `IsCancelled=true` when the
  item itself shows no cancellation.
- `CheckUser` refuses an empty VKN (İşNet would list every registered
  user) and returns `Registered=false` when İşNet answers `Result=Failed`
  with "bulunamadı".

## Status mapping

`ProviderCode` keeps the raw values (`Status/DetailStatus` for e-Fatura,
`Status/CancellationReportSendingStatus/ReportSendingStatus` for e-Arşiv).
`GIBCode` is the `DetailStatus` envelope code (1000–1300).

| İşNet e-Fatura `Status` | Status |
| --- | --- |
| `Onay_Bekliyor` (0), `Onay_Akisinda` (3) | delivered (outgoing), received (incoming) |
| `Onaylandi` (1), `Alici_Kabul_Etti` (24), `Otomatik_Onaylandi` (29), `Otomatik_Alici_Kabul_Etti` (30) | accepted, `ResponseCode` KABUL |
| `Reddedildi` (2), `Alici_Reddetti` (25) | rejected, RED |
| `Iade_Edildi` (4), `Alici_Iade_Etti` (26) | rejected, IADE |
| `Gonderildi` (8), `Gibe_Iletildi` (13) | from `DetailStatus` via the GİB table (1300 delivered, 1200 / 1220 sent, 11xx / 1210 / 1215 / 1230 failed, 1000 queued); 1235 `Fatura_Iptale_Konu_Edildi` cancelled |
| `Ziplendi` (12) | processing |
| `Imza_Bekliyor` (15) | draft |
| `Gib_Tarafinda_Hata_Olustu` (16), `Sistem_Hatasi` (22) | failed |
| `Uygulama_Yaniti_*` (36, 37), `Irsaliye_Yaniti_*` (41, 42), unknown values | from `DetailStatus`; else unknown |

Incoming documents that are not accepted, rejected, failed or cancelled are
`received`.

| İşNet e-Arşiv | Status |
| --- | --- |
| `Fatura_Hazirlaniyor` … `Pdf_Imzalaniyor` (1–4) | processing |
| `Fatura_Olusturuldu` (5) | delivered |
| `Silindi` (6) | cancelled |
| `IsCanceled=true` or `CancellationReportSendingStatus` other than `Iptal_Edilmedi` | cancelled |
| `ReportSendingStatus` 4 / 5 (GİB report errors) | unchanged; the text is in `StatusResult.Message` |

## Errors

[`errors.csv`](errors.csv) holds the status enumerations above, the
`Failed` code for `Result=Failed` answers (the raw `ErrorMessage` is kept
as `Message`), the `message:bulunamad` rule (→ not-found) and
`ip-not-allowlisted`. İşNet publishes no error-code list, so other
`Result=Failed` answers are `KindUnknown`. WCF faults (`a:InternalServiceFault`)
are `KindTransport` with the fault code and text. GİB envelope codes come
from the shared provider table.

## Sources

- **İşNet integration note `32-isnet.md`** (internal integration package,
  "İşNet (NetteFatura)"), with the common rules in `30-entegrator-genel.md`
  §5. It summarises İşNet's "E-Fatura" and "E-Arşiv" Web Servis Arayüz
  Dokümanları (V2, 12.08.2024), "AddressBook" (V1, 10.01.2023) and
  "Company" (V1, 16.12.2024), and İşNet's request samples.
- WSDL served by the İşNet test system, fetched without authentication and
  kept in [`testdata/wsdl`](testdata/wsdl):
  `https://einvoiceservicetest.isnet.net.tr/InvoiceService/ServiceContract/InvoiceService.svc?singleWsdl`,
  `https://einvoiceservicetest.isnet.net.tr/AddressBookService/ServiceContract/AddressBookService.svc?singleWsdl`.
- Public copies of the İşNet documents and request samples referenced by
  the note: <https://github.com/EfeSorogluu/NetteFatura-API>
  (`docs/official-docs`, `docs/request-samples`), and the community module
  the note cites for observed behaviour (IP filtering, `ETTN` spelling,
  "bulunamadı", units, viewer link):
  <https://github.com/mbrksntrk/dolibarr-isnet-nettefatura>.

## Open questions

- **Live access model.** How a read-only integrator gets its IP
  allowlisted (İşNet asks for successful test invoices first), and how each
  company authorises the IP for its VKN, are not documented.
- **IP failure shape.** "No TCP answer" comes from a community module.
  Through an HTTP proxy the same situation may surface as a reset during the
  TLS handshake (observed once from a non-allowlisted network, through a
  proxy, while preparing this module); resets are not classified as
  `ip-not-allowlisted` until confirmed.
- **Endpoints and SOAPAction values** come from community code; the
  SOAPActions match the test WSDL, the live URL is unconfirmed.
- **Response member spelling.** The note uses `Ettn` / `Taxpayertaxcode`,
  the WSDL `ETTN` / `TaxPayerTaxCode`; responses are read case-insensitively.
- **e-Arşiv items.** The note says the documented `ArchiveInvoice` table
  lacks ETTN, number and status; the WSDL has `ETTN`, `InvoiceNumber`,
  `Status`, `IsCanceled` and the report statuses. They are used when
  present, with the UBL and the `IsCancelled=true` query as fallbacks.
- **e-Arşiv enum values.** The note lists `ArchiveInvoiceReportSendingStatus`
  and `CanceledArchiveInvoiceReportSendingStatus` value 1 as
  `Gibe_Gonderilmek_Icin_Bekliyor`; the WSDL names it
  `Gibe_Otomatik_Raporlanacaktir`. Both are accepted.
- **ScenarioType spelling.** The note lists `TemelFatura`, `TicariFatura`, …;
  the WSDL `TEMELFATURA`, `TICARIFATURA`, …. `Profile` keeps the value as
  received.
- **Incoming counterparty.** Whether `Receiver` holds the sender on
  incoming items is unconfirmed; the UBL is authoritative.
- **Units.** Whether `IncludeMainCompany=true` alone returns invoices
  assigned to units. Set `VendorNumbers` to be safe.
- **PDF / HTML.** `IsPDFIncluded` / `IsHtmlIncluded` are in the documented
  request; the `InvoicePdf` / `InvoiceHtml` members come from the WSDL only.
- **`InvoiceCreationDate`** on incoming invoices: arrival at İşNet or the
  invoice's own creation time.
- **`GetDocumentViewerLink`** may return an `http://…:80` address that only
  answers over https, and may point to PDF or XML; it is returned unchanged.
- **Error texts.** Real `Result=Failed` messages (for example for a VKN the
  IP is not authorised for) and rate limits are not documented.

## Tests

```bash
go test ./...                                   # httptest fixtures
ISNET_VKN=… go test -tags live -run Live ./...  # read-only calls, test system
ISNET_VKN=… ISNET_LIVE=1 go test -tags live -run Live ./...
```

Fixture tests compare the generated requests with the samples in
[`testdata/fixtures`](testdata/fixtures), check member order and that every
element exists in the WSDL, and exercise the IP-allowlist path with a
listener that accepts connections and never answers. Optional live
variables: `ISNET_ENDPOINT`, `ISNET_ADDRESSBOOK_ENDPOINT`,
`ISNET_CHECK_VKN`, `ISNET_DAYS`, `ISNET_STATUS_UUID`,
`ISNET_EARCHIVE_UUID`.
