# provider/sovos

Sovos Türkiye Bulut (formerly FIT / Foriba) client for the
[`provider`](../README.md) contract: e-Fatura through
`ClientEInvoiceServices` and e-Arşiv through `ClientEArsivServices`
(SOAP 1.1, HTTP Basic authentication).

```bash
go get github.com/furkanmeclis/go-ubltr/provider/sovos
```

Digital Planet, which Sovos also owns, has a separate API and is not
covered by this module.

## Configuration

```go
c, err := sovos.New(sovos.Config{
	Username: "ws-user", Password: "secret",
	VKN:          "1234567801",
	SenderAlias:  "urn:mail:defaultgb@firma.com", // GB etiketi
	PostboxAlias: "urn:mail:defaultpk@firma.com", // PK etiketi
	Test:         true,
})
```

| Field | Notes |
| --- | --- |
| `Username`, `Password` | Web-service user, sent as `Authorization: Basic`. Sovos issues passwords for three months. |
| `EArchiveUsername`, `EArchivePassword` | e-Arşiv user (defaults to the above). |
| `VKN` | `VKN_TCKN` / `senderID` / `vkn`. |
| `SenderAlias` | GB alias: `SenderIdentifier` on send, `Identifier` for outgoing lists, status and downloads. |
| `PostboxAlias` | PK alias for incoming lists and downloads. |
| `Branch` | e-Arşiv `customizationParams` `BRANCH` (empty = the `default` branch). |
| `Test` | `https://efaturawstest.fitbulut.com/ClientEInvoiceServices/ClientEInvoiceServicesPort.svc` and `https://earsivwstest.fitbulut.com/ClientEArsivServicesPort.svc`. |
| `EInvoiceURL`, `EArchiveURL` | Required for live use; Sovos gives the live addresses with the account. Either may be empty to use one product only. |

Registry `Options`: `sender_alias`, `postbox_alias`, `branch`,
`earchive_endpoint`, `earchive_username`, `earchive_password`;
`provider.Config.Endpoint` is the e-Fatura URL.

Sovos blocks `?wsdl` on its servers and monitors call volume: it asks for
batched calls, at most two status queries per envelope per day and a
5-minute client timeout. Set a suitable `HTTP` client.

## Capabilities

| Operation | e-Fatura | e-Arşiv | Sovos method |
| --- | --- | --- | --- |
| Send | ✓ | ✓ | `sendUBL` (`DocType` `INVOICE`) / `sendInvoice` |
| Status (outgoing) | ✓ | ✓ | `getUBLList` + `getEnvelopeStatus` / `getStatus` |
| ListInbox | ✓ | – | `getUBLList` (`INBOUND`) |
| ListOutbox | ✓ | – | `getUBLList` (`OUTBOUND`) |
| Fetch UBL | ✓ in and out | ✓ signed XML | `getUBL` (`ZIP`) / `getSignedInvoice` |
| Fetch PDF / HTML | ✓ in and out | ✓ | `getInvoiceView` / `getInvoiceDocument` |
| CancelEArchive | – | ✓ | `getSignedInvoice` + `cancelInvoice` |

`SignsDocuments` and `AcceptsRawUBL` are true: Sovos removes any signature
nodes and signs with its own mali mühür. Not implemented: envelopes and
batches, commercial answers (`APP_RESP` needs an ApplicationResponse
document), recipient lookups (Sovos only offers the full GİB user list via
`getPartialUserList`), acknowledgements (there are none), credits.

Behaviour worth knowing:

- e-Fatura `Send` posts one invoice without an envelope, zipped as
  `<UUID>.xml`, so `Document.ReceiverAlias` (PK) and a GB alias are
  required. The envelope UUID is returned in `SendResult.EnvelopeUUID`,
  `CustInvID` (when Sovos numbers invoices) in `ProviderCode`.
- `Status` looks up the envelope of the invoice with `getUBLList`, then
  reads `getEnvelopeStatus`. `ResponseCode` is the GİB system response code
  (`GIBCode`), with Sovos' own `1000` (being processed) and `1100` (sent to
  GİB, no answer yet). KABUL / RED is not reported: Sovos asks not to poll
  `getInvResponses`.
- `ListInbox` / `ListOutbox` accept at most one day per call. The query is
  split into one-day windows; `Page.Next` is the RFC 3339 start of the next
  window. Zero `From` / `To` mean the last 24 hours. `OnlyNew` returns
  `ErrInvalidRequest`.
- e-Arşiv `Send` is synchronous; `SendResult.ID` is the number Sovos
  assigned and the status comes from the `Detail` prefix (`130:` signed).
- `CancelEArchive` needs the invoice number and tax-exclusive total for
  `cancelInvoice`; they are read from the signed XML
  (`cbc:ID`, `LegalMonetaryTotal/TaxExclusiveAmount`) and the cancel date is
  today. Use `Client.CancelInvoice` to pass them yourself. The reason is
  not sent (no such field).

## Status mapping

e-Fatura: `getEnvelopeStatus` `ResponseCode` through the GİB table (`1000`
queued, `1100` processing, `1200`/`1210`/`1220` sent, `1300` delivered,
other codes failed). e-Arşiv `statusCode` (EK3 status list):

| Code | Status |
| --- | --- |
| 100, 105, 120, 125, 126 | processing |
| 130 signed, 150 reported, 998 | delivered |
| 999 | failed |
| 400, 460 | cancelled |

## Errors

[`errors.csv`](errors.csv) holds the documented e-Fatura `ProcessingFault`
codes (`ef.*`, 1000–3960), e-Arşiv error codes (`ea.*`, 5–99) and e-Arşiv
statuses (`eastatus.*`). The prefixes only keep the ranges apart;
`Error.Code` is the raw number. Fault details are parsed from the SOAP fault
(`ProcessingFault`/`processingFaultType`) and e-Arşiv `preCheckError`
entries. HTTP 401 is `ErrAuth`; `ef.1300` (call limit) is `ErrRateLimit`.

## Sources

- Sovos API documentation and packages:
  <https://api.fitbulut.com/servis/> — "Sovos Bulut e-Fatura WS API v2.3"
  and "Sovos Bulut e-Arşiv Fatura WS API v2.3" zips (user guides,
  `ClientEInvoiceServices-2.2.wsdl`, `ClientEInvoiceServicesTypes-2.2.xsd`,
  `EArchiveInvoiceService_v2.wsdl`, SOAP request/response samples, error
  code tables, e-Arşiv status list EK3).
- Sovos sample client (endpoints, Basic auth usage):
  [Sovos-Compliance/turkey-cloud-sample-api-client](https://github.com/Sovos-Compliance/turkey-cloud-sample-api-client).
- Mirror of the technical documents:
  [ahmeti/e-belge-dokuman](https://github.com/ahmeti/e-belge-dokuman).

Open points: the published namespace is `http:/fitcons.com/eInvoice/`
(single slash) and is used as is. `getStatus` of the e-Arşiv service is in
the WSDL and SOAP samples but not in the user guide text.

## Tests

```bash
go test ./...                                   # httptest fixtures
SOVOS_USERNAME=… SOVOS_PASSWORD=… SOVOS_VKN=… SOVOS_GB=… SOVOS_PK=… \
  go test -tags live -run Live ./...            # read-only calls on the Sovos test system
```

Optional: `SOVOS_STATUS_UUID`, `SOVOS_EARCHIVE_UUID`.
