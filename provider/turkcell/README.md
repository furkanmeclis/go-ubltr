# provider/turkcell

Turkcell e-Şirket (efaturaservice) e-Fatura and e-Arşiv REST client for
the [`provider`](../README.md) contract. Standard library only.

```bash
go get github.com/furkanmeclis/go-ubltr/provider/turkcell
```

## Sources

- API reference (Redoc / OpenAPI): <https://developer.turkcellesirket.com/fatura>
- Authentication: <https://developer.turkcellesirket.com/pages/authentication.html>
- Rate limits: <https://developer.turkcellesirket.com/pages/rate-limits.html>
- Public Postman collection (request shapes, `pageIndex` / date format):
  <https://github.com/ePlatform/postman-collections/tree/master/efaturaservice>

## Configuration

```go
c, err := turkcell.New(turkcell.Config{
	APIKey: os.Getenv("TURKCELL_API_KEY"),
	Test:   true, // https://efaturaservicetest.isim360.com
})
```

| Field | Meaning |
| --- | --- |
| `APIKey` | Key from the portal's API Yönetimi page (test: `portaltest.isim360.com`). Sent as `x-api-key`. |
| `Test` | Uses `https://efaturaservicetest.isim360.com` instead of `https://efaturaservice.turkcellesirket.com`. |
| `Endpoint` | Overrides the base URL (scheme + host; `/v1` or `/v2` is added per endpoint). |
| `StandardXSLT` | `isStandartXslt` path value for PDF / HTML downloads (default `false`: the account's default XSLT). |
| `HTTP` | Any `provider.Doer`; defaults to `http.DefaultClient`. |

Through the registry (`import _ ".../provider/turkcell"`):
`provider.Open("turkcell", provider.Config{APIKey: k, Test: true, Options: map[string]string{"standard_xslt": "true"}})`.

The OAuth bearer-token flow (`/v1/token` with a service user) is not
implemented; the API key covers the same endpoints.

## Capabilities

| Operation | e-Fatura | e-Arşiv | Endpoint |
| --- | --- | --- | --- |
| send | yes | yes | `POST /v2/outboxinvoice` (`appType=1`), `POST /v2/earchive`; multipart `invoiceFile`, `status=20` |
| status | yes | yes | `GET /v2/outboxinvoice/{id}/status`, `GET /v2/inboxinvoice/{id}/status`, `GET /v2/earchive/{id}/status` |
| list-inbox | yes | – | `GET /v1/inboxinvoice/list` |
| fetch (ubl, pdf, html) | yes | yes | `GET /v2/{outboxinvoice,inboxinvoice,earchive}/{id}/ubl`, `…/pdf/{isStandartXslt}`, `…/html/{isStandartXslt}` |
| respond | yes | – | `POST /v1/invoiceresponse` (`Status` 1 Kabul / 2 Red) |
| cancel-earchive | – | yes | `PUT /v1/earchive/cancelinvoice` (GUID array) |
| mark-taken | yes (inbound) | – | `PUT /v1/inboxinvoice/updateisnew` (`IsNew=false`) |

`{id}` is the ETTN (the OpenAPI `id` parameter is described as "unique
id'si (ETTN)").

`Protocol: rest`, `AcceptsRawUBL`, `SignsDocuments`, `AssignsNumbers`,
`TestEndpoint`, `MaxBatch: 1`, `MinPollInterval: 150s` (inbox listing is
limited to 6 calls per 15 minutes).

Not offered because the documented API has no matching endpoint:
list-outbox (only "Arayüzden Kesilen" listings), check-user (only the full
GİB user list as a zip), credits.

Notes:

- `useManualInvoiceId` is `true` when the XML has `cbc:ID`; otherwise the
  system assigns the number and returns it as `SendResult.ID`.
- `Document.ReceiverAlias` is sent as `targetAlias` (e-Fatura).
  `Document.Email` sets `sendEMail=true` and `eMailAddress` (e-Arşiv).
- `ListQuery.OnlyNew` sends `isNew=true`; `From` / `To` map to
  `startDate` / `endDate` (`2006-01-02 15:04:05`). `Cursor` is the
  1-based `pageIndex`; `Limit` the page size (default 100, max 500).
- e-Arşiv cancellation works only for status 60 and takes no reason.

## Status mapping

Outgoing `status` codes go through [`errors.csv`](errors.csv): 0 draft,
20 queued, 30 processing, 40 failed, 50 sent, 60 / 61 / 62 / 70 / 81 / 82
delivered, 65 accepted, 80 / 85 rejected, 99 / 100 cancelled. A failing
`envelopeStatus` (GİB code, e.g. 1230) turns the result to failed;
`envelopeStatus` is reported as `GIBCode`.

Incoming `status`: 65 accepted, 80 / 85 rejected, 99 cancelled, 40
failed, anything else received. The raw value stays in `ProviderCode`.

## Errors

The API publishes no error-code list. Errors are classified by HTTP
status (401 auth, 403 permission, 404 not found, 422 validation — the
docs say not to resend unchanged — 429 rate limit, 5xx unavailable). The
422 body `{"Error": {"title", "detail"}}` becomes the message. No
`Retry-After` header is documented.

## Live tests

```bash
TURKCELL_API_KEY=... go test -tags live ./...
# optional: TURKCELL_ENDPOINT, TURKCELL_STATUS_UUID, TURKCELL_SEND_XML=/path/invoice.xml
```

## Open points

- Status 60 "Onaylandı" is used both for successful TEMELFATURA delivery
  and for an accepted TICARIFATURA; without the profile it maps to
  delivered.
- The `startDate` / `endDate` parameter names appear in the OpenAPI with
  their format appended (`startDate - (2019-10-30 00:00:00)`); the plain
  names come from the Postman collection.
- `items` in the inbox list is typed `object` in the OpenAPI; both an
  array and a single object are accepted.
- No response examples are published; fixtures follow the schemas.
