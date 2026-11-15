# Integrator providers

Integrator clients are nested modules under `provider/<name>` that implement
the vendor-neutral contract in [`provider`](../provider/README.md).

## Uyumsoft

Module `github.com/furkanmeclis/go-ubltr/provider/uyumsoft`
([README](../provider/uyumsoft/README.md)). It is a SOAP 1.1 client for the
Uyumsoft e-Fatura / e-Arşiv `Integration` service, hand-written from the
published WSDL. It also covers muhasebeizleme's `BasicIntegration` service,
which uses the same contract.

```go
import "github.com/furkanmeclis/go-ubltr/provider/uyumsoft"

c, err := uyumsoft.New(uyumsoft.Config{Username: u, Password: pw, Test: true})
// or: provider.Open("uyumsoft", …) / provider.Open("muhasebeizleme", …)
```

- **Presets**: `uyumsoft.Uyumsoft` sends a WS-Security UsernameToken and
  calls `efatura[-test].uyumsoft.com.tr/Services/Integration`.
  `uyumsoft.Muhasebeizleme` sends a `userInfo` element carrying the API key
  and calls `[test.]api.muhasebeizleme.com/Services/BasicIntegration.svc`.
  `Config.Endpoint` overrides the URL.
- **Operations**: e-Fatura and e-Arşiv send (raw UBL), status (in/out), inbox
  and outbox listing, UBL/PDF/HTML download, KABUL/RED, e-Arşiv cancel,
  user and alias lookup, mark taken, and credits. e-İrsaliye is not covered.
- **Errors**: SOAP faults and `IsSucceded="false"` messages map onto
  `provider.ErrorKind` through the embedded `errors.csv` and keyword rules.
  GİB envelope codes come from `provider.GIBCodes`.
- **Live tests**: `UYUMSOFT_USERNAME=… UYUMSOFT_PASSWORD=… go test -tags live -run Live ./...`
  inside `provider/uyumsoft`.
# Integrator modules

Each özel entegratör client is a nested module under `provider/<name>`
implementing the [`provider`](../provider/README.md) contract. Import the
module (blank import is enough) to register it, then open it by name or
use its typed constructor.

## nes

NES (nes.com.tr) REST API, e-Fatura and e-Arşiv.
Module `github.com/furkanmeclis/go-ubltr/provider/nes`, details in
[`provider/nes/README.md`](../provider/nes/README.md).

- Auth: API key from the NES portal, `Authorization: Bearer`.
- Hosts: `https://api.nes.com.tr` (live), `https://apitest.nes.com.tr`
  (`Test: true`); services `/einvoice`, `/earchive`, `/general`.
- Send: raw or zipped UBL-TR XML (multipart `File`, max 1 MB, zipped
  automatically when larger).
- Operations: e-Fatura send, status, list-inbox, list-outbox, fetch
  (UBL / PDF / HTML), respond, check-user, mark-taken, credits; e-Arşiv
  send, status, list-outbox, fetch, cancel-earchive, credits.
- Errors: 400 `invalidFields` and 422 `errors[].code` bodies, codes in
  `provider/nes/errors.csv`.
- Live tests: `NES_API_KEY=... go test -tags live ./...` in `provider/nes`.

## nilvera

Nilvera REST API, e-Fatura and e-Arşiv.
Module `github.com/furkanmeclis/go-ubltr/provider/nilvera`, details in
[`provider/nilvera/README.md`](../provider/nilvera/README.md).

- Auth: API key from the Nilvera portal, `Authorization: Bearer`.
- Hosts: `https://api.nilvera.com` (live), `https://apitest.nilvera.com`
  (`Test: true`); services `/einvoice`, `/earchive`, `/general`.
- Send: raw or zipped UBL-TR XML (`POST /einvoice/Send/Xml`,
  `POST /earchive/Send/Xml`, multipart `file`).
- Operations: e-Fatura send, status, list-inbox, list-outbox, fetch
  (UBL / PDF / HTML), respond, check-user, mark-taken, credits; e-Arşiv
  send, status, list-outbox, fetch, cancel-earchive, credits.
- Errors: `Errors[].Code` from the published 404 / 409 / 422 tables, in
  `provider/nilvera/errors.csv`.
- Live tests: `NILVERA_API_KEY=... go test -tags live ./...` in
  `provider/nilvera`.

## turkcell

Turkcell e-Şirket efaturaservice REST API, e-Fatura and e-Arşiv.
Module `github.com/furkanmeclis/go-ubltr/provider/turkcell`, details in
[`provider/turkcell/README.md`](../provider/turkcell/README.md).

- Auth: API key from the portal's API Yönetimi page, `x-api-key` header.
- Hosts: `https://efaturaservice.turkcellesirket.com` (live),
  `https://efaturaservicetest.isim360.com` (`Test: true`); `/v1` or `/v2`
  per endpoint as documented.
- Send: raw UBL-TR XML (`POST /v2/outboxinvoice`, `POST /v2/earchive`,
  multipart `invoiceFile`); numbers are assigned when the XML has no
  `cbc:ID`.
- Operations: e-Fatura send, status, list-inbox, fetch (UBL / PDF / HTML),
  respond, mark-taken; e-Arşiv send, status, fetch, cancel-earchive. No
  outbox listing, user check or credit endpoint is documented.
- Errors: classified by HTTP status (no vendor code list is published);
  invoice status codes in `provider/turkcell/errors.csv`. Inbox listing is
  rate limited to 6 calls per 15 minutes.
- Live tests: `TURKCELL_API_KEY=... go test -tags live ./...` in
  `provider/turkcell`.
Integrator modules implement the vendor-neutral
[`provider`](../provider/README.md) contract. Each is a separate module;
importing it registers it for `provider.Open`.

## İzibiz

Module `github.com/furkanmeclis/go-ubltr/provider/izibiz`, registry name
`izibiz`. SOAP: AuthenticationWS, EInvoiceWS, EIArchiveWS, EIrsaliyeWS.

```go
import _ "github.com/furkanmeclis/go-ubltr/provider/izibiz"

p, err := provider.Open("izibiz", provider.Config{Username: u, Password: pw, Test: true})
```

| Kind | Operations |
| --- | --- |
| e-Fatura | send, status, list-inbox, list-outbox, fetch (UBL/PDF/HTML), respond, mark-taken, check-user |
| e-Arşiv | send, status, fetch (UBL/PDF/HTML), cancel-earchive |
| e-İrsaliye | send, status |

- Test system: `https://efaturatest.izibiz.com.tr`. The production host is
  not published; pass `Endpoint`.
- One session is shared and renewed on session errors (`10004`, `2005`).
- Error codes from the İzibiz Developer Portal are in
  [`provider/izibiz/errors.csv`](../provider/izibiz/errors.csv).
- Live tests: `IZIBIZ_USERNAME`, `IZIBIZ_PASSWORD`, `-tags live`.

Details: [`provider/izibiz/README.md`](../provider/izibiz/README.md).

## EDM Bilişim

Module `github.com/furkanmeclis/go-ubltr/provider/edm`, registry name
`edm`. SOAP: `EFaturaEDM.svc`.

```go
import _ "github.com/furkanmeclis/go-ubltr/provider/edm"

p, err := provider.Open("edm", provider.Config{Username: u, Password: pw, VKN: vkn, Test: true})
```

| Kind | Operations |
| --- | --- |
| e-Fatura | send, status, list-inbox, list-outbox, fetch (UBL/PDF/HTML), mark-taken, check-user |

- Test system: `https://test.edmbilisim.com.tr/EFaturaEDM21ea/EFaturaEDM.svc`.
  Production URL comes with the contract (`edm.PortalEndpoint` and
  `edm.InteraktifEndpoint` are the public services).
- EDM publishes no error-code list; fault codes keep their raw value.
  [`provider/edm/errors.csv`](../provider/edm/errors.csv) holds the status
  texts used for status mapping.
- Commercial responses, e-Arşiv and e-İrsaliye are not covered: their
  values are not publicly documented.
- Live tests: `EDM_USERNAME`, `EDM_PASSWORD`, `-tags live`.

Details: [`provider/edm/README.md`](../provider/edm/README.md).
