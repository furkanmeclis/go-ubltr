# provider/trendyol

Trendyol E-Faturam client for the [`provider`](../README.md) contract,
using the public REST API documented at
<https://developers.trendyolefaturam.com>. Documents are sent as UBL-TR XML
through the `*-by-file` endpoints, so they can be built and validated with
go-ubltr first.

```bash
go get github.com/furkanmeclis/go-ubltr/provider/trendyol
```

## Configuration

API user (bireysel entegrasyon), acting for its own company:

```go
c, err := trendyol.New(trendyol.Config{
	Email: "api@firma.com", Password: "secret",
	UserID: 123, CompanyID: 456, // given by Trendyol for the account
	Test: true,                  // stage gateway (IP allow-listed)
})
```

Marketplace integrator (pazaryeri entegratörü), acting for a customer:

```go
c, err := trendyol.New(trendyol.Config{
	Email: "partner@entegrator.com", Password: "secret",
	CustomerEmail: "musteri@firma.com", CustomerPassword: "secret", CustomerTaxID: "1234567890",
})
```

| Field | Notes |
| --- | --- |
| `Email`, `Password` | `POST /api/auth/signin`; the token comes back in the `x-access-token` header. |
| `CustomerEmail`, `CustomerPassword`, `CustomerTaxID` | Marketplace mode: `customerSignIn` returns the customer token, `userId` and `companyId`. |
| `UserID`, `CompanyID` | Required for API users; ignored in marketplace mode. |
| `Source` | `source` of created documents (default `PARTNER`). |
| `InvoicePrefix` | `prefix` sent with `autoInvoiceId` when `Document.ID` is empty. |
| `Test` / `BaseURL` | Stage `https://stage-apigateway.trendyolefaturam.com`, live `https://apigateway.trendyolecozum.com` (default). |

Registry: `provider.Config.Username` / `Password` are the sign-in e-mail and
password, `Endpoint` the base URL; `Options`: `user_id`, `company_id`,
`customer_email`, `customer_password`, `customer_tax_id` (defaults to
`VKN`), `source`, `invoice_prefix`.

Session: the client signs in on first use and signs in again once when a
call returns HTTP 401. The documentation names the `x-access-token`
response header but not how to send it back, so requests carry both
`x-access-token: <token>` and `Authorization: Bearer <token>`.

## Capabilities

| Operation | e-Fatura | e-Arşiv | Endpoint |
| --- | --- | --- | --- |
| Send (UBL file) | ✓ | ✓ | `POST /api/invoice/documents/outgoing-einvoice-by-file`, `/earchive-by-file` |
| Status (outgoing) | ✓ | ✓ | `GET …/outgoing-einvoice/status/{uuid}`, `…/earchive/status/{uuid}` |
| ListInbox | ✓ | – | `POST …/incoming-einvoice/search` |
| Fetch UBL / PDF / HTML | ✓ in and out | ✓ | `POST …/documents/download` (temporary link) |
| Respond (KABUL / RED) | ✓ | – | `POST …/documents/sent-reply` |
| CheckUser | ✓ | – | `GET /api/invoice/taxpayers/{taxId}` |
| CancelEArchive | – | ✓ | `POST …/earchive/cancel` |

`AssignsNumbers` (via `autoInvoiceId`), `SignsDocuments` and
`AcceptsRawUBL` are true. Not implemented: JSON (non-UBL) document
creation, e-İrsaliye, re-send, permanent links, partner credit endpoints.

Behaviour worth knowing:

- `Send` posts `invoiceFile` (the XML) and `additionalInformation` as a JSON
  part (the OpenAPI default encoding for an object in
  `multipart/form-data`). `localReferenceId` is the ETTN; `targetAlias` is
  `Document.ReceiverAlias` (needed when the receiver has several aliases).
- `ListInbox` maps `From` / `To` to `startDate` / `endDate` (dates) and
  `Limit` to the page size (default 50). The page index base is not
  documented: `Page.Next` is `currentPage+1` while pages come back full.
  `OnlyNew` returns `ErrInvalidRequest` (there is no acknowledgement call).
  Amounts are kuruş in the API and are formatted as decimal strings.
- `Fetch` asks for a temporary link and downloads it without credentials.
- `CheckUser` treats HTTP 404 as "not registered"; deleted aliases are
  skipped.
- e-Arşiv `Status` only reports `gibStatus` (`REPORTED`,
  `READY_TO_BE_REPORTED`); a found invoice is `StatusDelivered`.

## Status mapping

From "Fatura Statü Kodları" (`status`, raw value in `ProviderCode`;
`gibStatusCode` in `GIBCode`):

| Code | Meaning | Status |
| --- | --- | --- |
| 10, 20 | İşleniyor, Doküman Hazırlanıyor | processing |
| 29 | Doküman Hatası | failed |
| 30 | Oluşturuldu | queued |
| 40 | GİB'e Gönderildi | sent |
| 50, 100, 200 | Yanıt Bekleniyor, Reddediliyor, Onaylanıyor | delivered |
| 105 | Reddedildi | rejected (`ResponseCode` RED) |
| 205 | Onaylandı (final for e-Arşiv, temel and accepted ticari) | delivered |
| 305 | İptal Edildi | cancelled |
| 405 | Hatalı | failed |

205 is mapped to delivered because the same code is final for invoices
that need no answer; listing results use `replyType` (APPROVED / DENIED)
to report accepted / rejected.

## Errors

Errors are RFC 7807 problem documents; `Error.Code` is `code` or `title`,
`Error.Message` is `detail`. [`errors.csv`](errors.csv) maps the documented
HTTP statuses (401 auth, 403 permission (tax ID mismatch, IP not allowed),
404 not-found, 409 invalid-state, 422 validation, 500 unavailable) and the
status codes above.

## Sources

- Trendyol E-Faturam API documentation (OpenAPI):
  <https://developers.trendyolefaturam.com/OpenApi/trendyol-e-faturam-entegrasyon-dokumani>
  and the "Enum ve Kod Açıklamaları" pages.
- Cross-checked with [vahaponur/ty-efaturam-go](https://github.com/vahaponur/ty-efaturam-go).

Open points: the token header for requests is not documented (both
headers are sent); the page index base of the search endpoint is not
documented.

## Tests

```bash
go test ./...                                        # httptest fixtures
TRENDYOL_EMAIL=… TRENDYOL_PASSWORD=… TRENDYOL_USER_ID=… TRENDYOL_COMPANY_ID=… \
  go test -tags live -run Live ./...                 # read-only calls on the stage gateway
```

Marketplace mode: set `TRENDYOL_CUSTOMER_EMAIL`, `TRENDYOL_CUSTOMER_PASSWORD`,
`TRENDYOL_CUSTOMER_TAX_ID` instead of the ids. Optional `TRENDYOL_CHECK_VKN`.
