# provider/nes

NES (nes.com.tr) e-Fatura and e-Arşiv REST client for the
[`provider`](../README.md) contract. Standard library only.

```bash
go get github.com/furkanmeclis/go-ubltr/provider/nes
```

## Sources

- e-Fatura OpenAPI: <https://apitest.nes.com.tr/einvoice/index.html> (`v1.swagger.json`)
- e-Arşiv OpenAPI: <https://apitest.nes.com.tr/earchive/index.html> (`v1.swagger.json`)
- General OpenAPI (credits): <https://apitest.nes.com.tr/general/v1/swagger.json>
- Developer portal: <https://developertest.nes.com.tr/docs> — API key
  (`/docs/auth/auth-token`), quick start (`/docs/fast-integration`), error
  format (`/docs/shared-classes`)

## Configuration

```go
c, err := nes.New(nes.Config{
	APIKey:      os.Getenv("NES_API_KEY"),
	Test:        true,                            // https://apitest.nes.com.tr
	SenderAlias: "urn:mail:defaultgb@firma.com", // default e-Fatura gönderici birim
	SourceApp:   "my-erp",                        // mandatory SourceApp field (default "go-ubltr")
})
```

| Field | Meaning |
| --- | --- |
| `APIKey` | Key created in the portal (Yönetim Paneli > API Tanımları). Sent as `Authorization: Bearer`. |
| `Test` | Uses `https://apitest.nes.com.tr` instead of `https://api.nes.com.tr`. |
| `Endpoint` | Overrides the base URL (scheme + host; `/einvoice`, `/earchive`, `/general` are appended). |
| `SourceApp` | Value of the mandatory `SourceApp` upload field. |
| `SenderAlias` | Used when `Document.SenderAlias` is empty; required for e-Fatura uploads. |
| `HTTP` | Any `provider.Doer`; defaults to `http.DefaultClient`. |

Through the registry (`import _ ".../provider/nes"`):
`provider.Open("nes", provider.Config{APIKey: k, Test: true, Options: map[string]string{"source_app": "erp", "sender_alias": "urn:mail:..."}})`.

On the live system an API key needs the scope of each endpoint it calls
(e.g. `einvoice:upload:write`); otherwise the API answers 403, reported as
`provider.ErrPermission`. Scopes are not enforced on the test system.

## Capabilities

| Operation | e-Fatura | e-Arşiv | Endpoint |
| --- | --- | --- | --- |
| send | yes | yes | `POST /{einvoice,earchive}/v1/uploads/document` (multipart, raw or zipped UBL-TR, `IsDirectSend=true`) |
| status | yes | yes | `GET /einvoice/v1/{outgoing,incoming}/invoices/{uuid}`, `GET /earchive/v1/invoices/{uuid}` |
| list-inbox | yes | – | `GET /einvoice/v1/incoming/invoices` |
| list-outbox | yes | yes | `GET /einvoice/v1/outgoing/invoices`, `GET /earchive/v1/invoices` |
| fetch (ubl, pdf, html) | yes | yes | `…/{uuid}/xml`, `…/{uuid}/pdf`, `…/{uuid}/html` |
| respond | yes | – | `POST /einvoice/v1/incoming/invoices/{uuid}/documentAnswer` |
| cancel-earchive | – | yes | `POST /earchive/v1/invoices/cancel` |
| check-user | yes | – | `GET /einvoice/v1/users/{vkn}/All` (404 → `Registered: false`) |
| mark-taken | yes (inbound) | – | `PUT /einvoice/v1/incoming/invoices/bulk/Transferred` |
| credits | yes | yes | `GET /general/v1/management/creditsummary` |

`Protocol: rest`, `AcceptsRawUBL`, `SignsDocuments`, `TestEndpoint`,
`MaxBatch: 1` (one file per upload). Webhooks and numbering are not
declared (see open points).

Notes:

- The upload `File` field is limited to 1 MB. Larger XML is zipped
  automatically; if the zip is still too large, `Send` returns
  `ErrValidation` without calling the API.
- `ListQuery.OnlyNew` sends `erpTransferred=False`; `MarkTaken` sets the
  documents to `Transferred`. `Cursor` is the page number; `Limit` is the
  page size (default 50). Listings are sorted `CreatedAt desc`.
- The e-Arşiv cancel endpoint takes no reason; the `reason` argument is
  ignored.
- `Credits.Remaining` is `totalDefinedCount - totalUsedCount -
  totalExpiredUnUsedCount`.

## Status mapping

e-Fatura `outgoingStatus`:

| NES value | Status |
| --- | --- |
| `None`, `Waiting` | queued |
| `WaitingToBeSigned`, `WaitingToBeEnvelopeCreate`, `WaitingToBeSend`, `EnvelopeIsWaitingToBeProcessedByGib` | processing |
| `EnvelopeIsWaitingToBeSendedToReceiverByGib`, `EnvelopeIsBeenWaitingToBeTransferredToReceiverByGib`, `EnvelopeSystemResponseIsBeingWaitedFromReceiver` | sent |
| `EnvelopeHasBeenTransferredToReceiverSuccessfully` | delivered |
| `ErrorHasBeenDetectedInEnvelopeByGib`, `EnvelopeCouldNotTransferredToReceiverByGib`, `ErrorHasBeenDetectedInEnvelopeByReceiver`, `FileNotFound`, `XmlParseError` | failed |
| `UnknownStatus` / other | unknown |

`documentAnswer` `Accepted` / `Rejected` overrides it with accepted /
rejected (`ResponseCode` KABUL / RED). The envelope `code` is reported as
`GIBCode` when it is a GİB code; the raw enum stays in `ProviderCode`.
Incoming documents are `received` until answered.

e-Arşiv `archiveDocumentStatus`: `Waiting` → queued, `WaitingSign` →
processing, `Signed` → sent (delivered once `reportDivisionStatus` is
`Completed`), `Error` / `XmlFileDoesNotExist` / `XmlParseError` → failed;
`isCanceled` → cancelled.

## Errors

The API answers 400 with `{"message", "invalidFields": [{field, description, detail}]}`
and 422 with `{"message", "errors": [{code, description, detail}]}`. The
HTTP status sets the kind (`provider.HTTPError`); a code found in
[`errors.csv`](errors.csv) refines it (e.g. `422-15` RECORD_EXISTS →
duplicate, `1150` → validation with `GIBCode` 1150). 409 on upload
("Numaralandırma çakışması") and on respond ("Daha önce cevaplanmış") is
`ErrDuplicate`. No rate limit or `Retry-After` header is documented.

## Live tests

```bash
NES_API_KEY=... go test -tags live ./...
# optional: NES_ENDPOINT, NES_SENDER_ALIAS, NES_CHECK_VKN, NES_SEND_XML=/path/invoice.xml
```

Live tests use the test system and only send a document when
`NES_SEND_XML` is set.

## Open points

- The live base URL `https://api.nes.com.tr` is taken from the Swagger UI
  hosted there (same OpenAPI as the test host); the portal only shows the
  test host.
- Whether NES assigns `cbc:ID` when it is empty (series definitions exist)
  is not documented for uploads, so `AssignsNumbers` is false.
- Notification rules (`/v1/notifications/.../dynamicrules`) are not
  documented as HTTP webhooks; `Webhooks` is false.
- e-Arşiv e-mail delivery on upload is not a documented upload field;
  `Document.Email` is ignored.
- Listing items expose `id` (format uuid) and no separate ETTN field; it is
  used as `DocumentInfo.UUID`, matching the `{uuid}` path parameters.
