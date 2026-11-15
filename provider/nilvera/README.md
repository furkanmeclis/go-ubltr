# provider/nilvera

Nilvera e-Fatura and e-Arşiv REST client for the
[`provider`](../README.md) contract. Standard library only.

```bash
go get github.com/furkanmeclis/go-ubltr/provider/nilvera
```

## Sources

- Overview, hosts and auth: <https://developer.nilvera.com/genel-bakis>
- API key: <https://developer.nilvera.com/api-anahtari-uretimi>
- Error codes: <https://developer.nilvera.com/hata-kodlari>
- Endpoint pages (each embeds its OpenAPI fragment; append `.md` for
  Markdown, index at <https://developer.nilvera.com/llms.txt>):
  `api/e-fatura-api/...`, `api/e-arsiv-api/...`, `api/genel-api/...`

## Configuration

```go
c, err := nilvera.New(nilvera.Config{
	APIKey: os.Getenv("NILVERA_API_KEY"),
	Test:   true, // https://apitest.nilvera.com
})
```

| Field | Meaning |
| --- | --- |
| `APIKey` | Key from the Nilvera portal (API Tanımları > Yeni Anahtar). Sent as `Authorization: Bearer`. Test and live keys differ. |
| `Test` | Uses `https://apitest.nilvera.com` instead of `https://api.nilvera.com`. |
| `Endpoint` | Overrides the base URL (scheme + host; `/einvoice`, `/earchive`, `/general` are appended). |
| `HTTP` | Any `provider.Doer`; defaults to `http.DefaultClient`. |

Through the registry (`import _ ".../provider/nilvera"`):
`provider.Open("nilvera", provider.Config{APIKey: k, Test: true})`.

## Capabilities

| Operation | e-Fatura | e-Arşiv | Endpoint |
| --- | --- | --- | --- |
| send | yes | yes | `POST /einvoice/Send/Xml?Alias=…`, `POST /earchive/Send/Xml` (multipart `file`, raw or zipped UBL-TR) |
| status | yes | yes | `GET /einvoice/Sale/{UUID}/Status`, `GET /einvoice/Purchase/{UUID}/Status`, `GET /earchive/Invoices/{UUID}/Status` |
| list-inbox | yes | – | `GET /einvoice/Purchase` |
| list-outbox | yes | yes | `GET /einvoice/Sale`, `GET /earchive/Invoices` |
| fetch (ubl, pdf, html) | yes | yes | `…/{UUID}/xml`, `…/{UUID}/pdf`, `…/{UUID}/html` |
| respond | yes | – | `POST /einvoice/Purchase/SendAnswer` (`approved` / `rejected`, `RejectNote`) |
| cancel-earchive | – | yes | `PUT /earchive/Invoices/{UUID}/Cancel` |
| check-user | yes | – | `GET /general/GlobalCompany/Check/TaxNumber/{TaxNumber}?globalUserType=Invoice` |
| mark-taken | yes (inbound) | – | `PUT /einvoice/Purchase/Operation/Transferred` |
| credits | yes | yes | `GET /general/Credits` (rows summed) |

`Protocol: rest`, `AcceptsRawUBL`, `SignsDocuments`, `TestEndpoint`,
`MaxBatch: 1`.

Notes:

- `Document.ReceiverAlias` becomes the `Alias` query parameter for
  e-Fatura; the sender alias is not an upload parameter.
- `ListQuery.OnlyNew` sends `IsTransfer=false`; `MarkTaken` sets the
  documents to `Transferred`. `From` / `To` filter on `CreatedDate`.
  `Cursor` is the page number; `Limit` the page size (default 50).
- The e-Arşiv cancel endpoint takes no reason; the argument is ignored.
- `Respond` sends `RejectNote` only for rejections.

## Status mapping

e-Fatura `InvoiceStatus.Code`: `waiting` → processing, `succeed` → sent,
`error` → failed. A known envelope `GIBCode` refines a non-error status via
the GİB table (1000 queued, 1200 / 1220 sent, 1300 delivered, failing codes
failed). `AnswerCode` `approved` and `documentAnsweredAutomatically` →
accepted (KABUL), `rejected` → rejected (RED). Incoming documents are
`received` until answered. The raw code stays in `ProviderCode`.

e-Arşiv: `CancelStatus` → cancelled; `waiting` → processing; `succeed` →
sent, delivered once `ReportStatus` is `Reported`; `error` → failed.

## Errors

Error bodies are `{"Message", "Errors": [{"Code", "Description", "Detail"}]}`.
The HTTP status sets the kind (`provider.HTTPError`); a code listed in
[`errors.csv`](errors.csv) (built from the 404 / 409 / 422 tables) refines
it, e.g. `2000` → `ErrQuotaExceeded`, `1001` → `ErrDuplicate`, `2026` →
`ErrInvalidState`, `3000` → `ErrRecipient`. Codes whose text only says an
internal step failed keep the HTTP kind. Nilvera codes are not GİB codes,
so `GIBCode` is zero on errors. No rate limit or `Retry-After` header is
documented.

## Live tests

```bash
NILVERA_API_KEY=... go test -tags live ./...
# optional: NILVERA_ENDPOINT, NILVERA_CHECK_VKN, NILVERA_SEND_XML=/path/invoice.xml, NILVERA_ALIAS
```

## Open points

- The 409 example on the error page uses code 1000 with the "ETTN
  Numarası Sistemde Kayıtlıdır" text, while the table lists 1000 as a
  VKN/TCKN mismatch and 1001 as the ETTN duplicate. The table is used.
- The 404 example uses 3006 "Belge Bulunamadı" while the table lists 3006
  as "Rapor Sistemde Bulunamadı"; both map to `ErrNotFound`.
- `GlobalCompanyInfoResponse.Name` / `Type` are read as alias name / type;
  the docs give no field descriptions.
- No upload size limit, numbering behaviour or webhook mechanism is
  documented; `AssignsNumbers` and `Webhooks` are false.
