# provider/qnb

QNB eSolutions (eFinans) client for the
[`provider`](../README.md) contract: e-Fatura through `connectorService`
and e-Arşiv through `EarsivWebService` (SOAP 1.1, JAX-WS).

```bash
go get github.com/furkanmeclis/go-ubltr/provider/qnb
```

## Configuration

```go
c, err := qnb.New(qnb.Config{
	Username: "ws-user", Password: "secret", // e-Fatura web-service user
	EArchiveUsername: "vkn.portaltest",       // optional, e-Arşiv user
	EArchivePassword: "secret",
	VKN:     "3250566851",
	ERPCode: "ERP1", // erpKodu from the QNB project team
	Test:    true,   // QNB test endpoints
})
```

Or by name with the registry:

```go
import _ "github.com/furkanmeclis/go-ubltr/provider/qnb"

p, err := provider.Open("qnb", provider.Config{
	Username: u, Password: pw, VKN: vkn, Test: true,
	Options: map[string]string{"erp_code": "ERP1", "earchive_username": eu, "earchive_password": epw},
})
```

| Field | Notes |
| --- | --- |
| `Username`, `Password` | Web-service user. QNB recommends a separate user that never logs in to the portal so its password does not expire. |
| `EArchiveUsername`, `EArchivePassword` | e-Arşiv user (defaults to the e-Fatura user). |
| `VKN` | Sent as `vergiTcKimlikNo` / `vkn`. |
| `ERPCode` | `erpKodu`; not needed for in-house integrations. |
| `Test` | `https://erpefaturatest1.qnbesolutions.com.tr/efatura/ws/connectorService` and `https://earsivtest.qnbesolutions.com.tr/earsiv/ws/EarsivWebService`. |
| `ConnectorURL`, `EArchiveURL` | Required for live use: QNB shares live endpoints after the test phase. Either may be left empty to use only one product. |
| `Branch`, `CashRegister` | e-Arşiv `sube` / `kasa` (default `DFLT`). |
| `InvoiceSeries` | Sent as `seriNo` / `faturaSeri` when `Document.ID` is empty so the portal numbers the invoice. |

Registry `Options`: `earchive_endpoint`, `earchive_username`,
`earchive_password`, `erp_code`, `branch`, `cash_register`,
`invoice_series`. `provider.Config.Endpoint` is the connector URL.

Authentication is a WS-Security `UsernameToken` header on every request
(the documentation's "SOAP Header" method); there is no session to keep.
QNB limits each WSDL to 300 requests per minute per source IP and answers
`HTTP 429` (mapped to `ErrRateLimit`) without `Retry-After`.

## Capabilities

| Operation | e-Fatura | e-Arşiv | QNB method |
| --- | --- | --- | --- |
| Send | ✓ | ✓ | `belgeGonderExt` / `faturaOlusturExt` |
| Status (outgoing) | ✓ | ✓ | `gidenBelgeDurumSorgulaExt` / `faturaSorgulaExt` |
| ListInbox | ✓ | – | `gelenBelgeleriListeleExt` |
| Fetch UBL / PDF / HTML | ✓ in and out | ✓ out | `gelenBelgeleriIndirExt`, `gidenBelgeleriIndir` / `faturaSorgulaExt` |
| MarkTaken (incoming) | ✓ | – | `belgelerAlindi` |
| CheckUser | ✓ | – | `efaturaKullaniciBilgisi` |
| Credits | ✓ | – | `kontorBilgisiGetir` (`FATURA`, `ADET`) |
| CancelEArchive | – | ✓ | `faturaIptalEt` |

`SignsDocuments` and `AcceptsRawUBL` are true: the UBL is sent unsigned and
QNB applies the mali mühür. Not implemented: commercial answers
(`UYGULAMA_YANITI_UBL` needs an ApplicationResponse document, which the
`provider.Response` type does not carry), outbox listing and e-İrsaliye.

Behaviour worth knowing:

- e-Fatura `Send` is asynchronous. `belgeNo` is set to the ETTN, as the
  documentation recommends, and the returned `belgeOid` is in
  `SendResult.ProviderCode`. `Status` queries with `belgeNoTipi=YEREL`
  (the ETTN) and falls back to `ETTN`, which only finds processed documents.
- `belgeHash` is the upper-case hex MD5 of the XML, as in the documentation
  sample.
- e-Arşiv `Send` is synchronous and returns `StatusDelivered` with the
  number QNB assigned in `SendResult.ID`; `Status` returns
  `StatusCancelled` once `iptalTarihi` is present. `CancelEArchive` ignores
  the reason (the service has no such field). gzip-compressed `belgeIcerigi`
  outputs are inflated.
- `ListInbox` pages with `sonAlinanBelgeSiraNumarasi` (`Page.Next` is the
  last `belgeSiraNo`, at most 100 rows per call). The service rejects date
  filters together with that cursor, so `From` / `To` are applied on the
  client to `faturaGelisTarihi`. `OnlyNew` sends `belgelerAlindiMi=false`.
- Outgoing e-Fatura `Fetch` first looks up the `belgeOid` with a status
  call, because `gidenBelgeleriIndir` takes OIDs.

## Status mapping

`StatusResult.ProviderCode` keeps the raw `durum`, `gonderimDurumu` and
`yanitDurumu`; `GIBCode` is `gonderimCevabiKodu` when it is a GİB code.

| QNB | Status |
| --- | --- |
| `durum` 1 Alındı | queued |
| `durum` 2 İşleme Hatası | failed (`aciklama` in `Message`) |
| `durum` 3, `gonderimDurumu` −2 | cancelled |
| `durum` 3, `gonderimDurumu` −1 / 0 / 1 | processing |
| `durum` 3, `gonderimDurumu` 2 / 3 | sent |
| `durum` 3, `gonderimDurumu` 4 | delivered |
| `gonderimCevabiKodu` ≥ 1000 | GİB table (`1300` delivered, `1163` failed, …) |
| `ulastiMi` true | at least delivered |
| `yanitDurumu` 1 / 2 | rejected / accepted (`ResponseCode` RED / KABUL) |

## Errors

[`errors.csv`](errors.csv) holds the documented status values, the e-Arşiv
result code `AE00000`, and SOAP fault texts used to reclassify the generic
`Exception` fault (`bulunamadı` → not-found, `Hash hatası` → validation,
`username.pwd.mismatch` → auth, `TPS_POSTA_KUTUSU_YETKI` → permission with
GİB 1172). Other e-Arşiv `resultCode` values are returned as
`KindUnknown` with the raw code and text; QNB does not publish the AE list.

## Sources

- QNB eSolutions API documentation:
  <https://www.qnbesolutions.com.tr/api-entegrasyon.html> (method tables,
  SOAP samples, status codes, rate limit, authentication).
- WSDL / XSD: `https://erpefaturatest1.qnbesolutions.com.tr/efatura/ws/connectorService?wsdl`
  (and `?xsd=1`), `https://earsivtest.qnbesolutions.com.tr/earsiv/ws/EarsivWebService?wsdl`
  (and `?xsd=1`).
- Open-source clients read for cross-checking:
  [Hasokeyk/qnb-finansbank-invoice-php](https://github.com/Hasokeyk/qnb-finansbank-invoice-php),
  [muhittingulap/qnbfinansbank](https://github.com/muhittingulap/qnbfinansbank),
  [coderserdar/QNBFinansGIB](https://github.com/coderserdar/QNBFinansGIB).

Open points: the documentation tables list `FATURA_UBL` as the `belgeTuru`
of `gidenBelgeDurumSorgulaExt`, while its SOAP samples send `FATURA`; this
module follows the samples. `kontorBilgisiGetir` is in the WSDL but not in
the documentation.

## Tests

```bash
go test ./...                                   # httptest fixtures
QNB_USERNAME=… QNB_PASSWORD=… QNB_VKN=… \
  go test -tags live -run Live ./...            # read-only calls on the QNB test system
```

Optional live variables: `QNB_ERP_CODE`, `QNB_EARCHIVE_USERNAME`,
`QNB_EARCHIVE_PASSWORD`, `QNB_CHECK_VKN`, `QNB_STATUS_UUID`.
