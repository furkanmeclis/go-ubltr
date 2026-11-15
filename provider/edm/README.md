# provider/edm

EDM Bilişim özel entegratör client for the [`provider`](../README.md)
contract. SOAP, `EFaturaEDM.svc` (SOAP 1.1, WCF).

```bash
go get github.com/furkanmeclis/go-ubltr/provider/edm
```

## Configuration

```go
import (
	"github.com/furkanmeclis/go-ubltr/provider"
	"github.com/furkanmeclis/go-ubltr/provider/edm"
)

p, err := edm.New(edm.Config{
	Username: os.Getenv("EDM_USERNAME"),
	Password: os.Getenv("EDM_PASSWORD"),
	VKN:      "1234567801", // sender VKN, sent as SENDER/@vkn
	Test:     true,         // edm.TestEndpoint
})

// or through the registry
p, err := provider.Open("edm", provider.Config{Username: u, Password: pw, VKN: vkn, Test: true})
```

| `Config` field | Registry option | Sent as |
| --- | --- | --- |
| `Endpoint` | `Config.Endpoint` | Full `EFaturaEDM.svc` URL. Required outside the test system. |
| `VKN` | `Config.VKN` | `SENDER/@vkn` |
| `ApplicationName` | `application_name` | `REQUEST_HEADER/APPLICATION_NAME` (default `go-ubltr`) |
| `ChannelName` | `channel_name` | `REQUEST_HEADER/CHANNEL_NAME` |
| `Hostname` | `hostname` | `REQUEST_HEADER/HOSTNAME` |
| `Reason` | `reason` | `REQUEST_HEADER/REASON` |

Known service URLs:

| Constant | URL |
| --- | --- |
| `TestEndpoint` | `https://test.edmbilisim.com.tr/EFaturaEDM21ea/EFaturaEDM.svc` (from the edm-efatura docs) |
| `PortalEndpoint` | `https://portal2.edmbilisim.com.tr/EFaturaEDM/EFaturaEDM.svc` |
| `InteraktifEndpoint` | `https://interaktif.edmbilisim.com.tr/EFaturaEDM/EFaturaEDM.svc` |

Which production URL an account uses comes with the EDM contract, so the
module does not pick one.

### Session

`Login` runs on the first call and its `SESSION_ID` is shared by all
goroutines. EDM does not document its session error, so a SOAP fault that
mentions the session (`session` / `oturum`) triggers one new login and a
retry. Call `Logout` when done.

## Capabilities

| Kind | Operation | EDM method |
| --- | --- | --- |
| `efatura` | send | `SendInvoice` (raw XML in `CONTENT`, `COMPRESSED=N`) |
| | status | `GetInvoiceStatus` |
| | list-inbox / list-outbox | `GetInvoice` (`HEADER_ONLY=Y`, `DIRECTION` IN/OUT, `READ_INCLUDED`) |
| | fetch (ubl, pdf, html) | `GetInvoice` (`INVOICE_CONTENT_TYPE` XML/PDF/HTML) |
| | mark-taken | `MarkInvoice` (`READ`) |
| | check-user | `CheckUser` |

`SignsDocuments` and `AcceptsRawUBL` are set: the edm-efatura client sends
unsigned UBL. `MaxBatch` and `MinPollInterval` are unknown (0).

Listings have no cursor and no date filter (passing either is an
`ErrInvalidRequest`); use `OnlyNew: true` with `MarkTaken`.

### Status mapping

`STATUS` arrives as `"STATUS - SUBSTATUS"`. The text, without spaces, is
looked up in [`errors.csv`](errors.csv) (from the edm-efatura status map):
for example `SEND-WAIT_APPLICATION_RESPONSE` → `delivered`,
`ACCEPTED-SUCCEED` → `accepted`, `REJECTED-SUCCEED` → `rejected`,
`RECEIVE-SUCCEED` → `received`, `SEND-FAILED` → `failed`. A failing
`GIB_STATUS_CODE` overrides an in-progress status.

## Errors

EDM reports errors as SOAP faults whose detail is `RequestFault`
(`ERROR_CODE`, `ERROR_SHORT_DES`, `ERROR_LONG_DES`), as declared in the
WSDL. EDM publishes no `ERROR_CODE` list: codes keep their raw value with
`KindUnknown`, except GİB codes (1000–1300), which use the GİB table. A
fault on `Login` is reported as `ErrAuth`.

## Sources

- Public WSDL, `?singleWsdl` of the three URLs above (copies in
  [`testdata/wsdl`](testdata/wsdl)). Request bodies produced by the tests
  validate against all three.
- `github.com/rebasesoftware/edm-efatura` (login, send, get invoice,
  status, request header fields, test URL, status meanings).

## Live tests

Live tests only read and use your own account:

```bash
EDM_USERNAME=... EDM_PASSWORD=... go test -tags live -run Live ./...
# optional: EDM_ENDPOINT=https://... EDM_VKN=<VKN for CheckUser> EDM_UUID=<ETTN>
```

## Not covered

These exist in the WSDL but their values or behaviour are not documented
publicly, so they are left out:

- Commercial response (`SendInvoiceResponseWithServerSign`): `STATUS`
  values are not specified.
- e-Arşiv (`SendInvoice` with `EARSIVFATURA`, `CancelInvoice`,
  `ArchiveInvoice`), e-İrsaliye, credits (`CheckCounter`).
- `OFFSET`, `START_DATE` / `CR_START_DATE` paging and date filters.
