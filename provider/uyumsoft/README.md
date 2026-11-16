# provider/uyumsoft

`provider.Provider` for the Uyumsoft e-Fatura / e-Arşiv **Integration** SOAP
service, and for services that expose the same contract (muhasebeizleme
**BasicIntegration**).

```bash
go get github.com/furkanmeclis/go-ubltr/provider/uyumsoft
```

Importing the package registers `uyumsoft` and `muhasebeizleme` with
`provider.Register`.

## Configuration

```go
c, err := uyumsoft.New(uyumsoft.Config{
	Username: os.Getenv("UYUMSOFT_USERNAME"),
	Password: os.Getenv("UYUMSOFT_PASSWORD"),
	Test:     true, // efatura-test.uyumsoft.com.tr
})
res, err := c.Send(ctx, &provider.Document{XML: ubl, ReceiverAlias: "urn:mail:defaultpk@alici.com.tr"})
```

or through the registry:

```go
import _ "github.com/furkanmeclis/go-ubltr/provider/uyumsoft"

p, err := provider.Open("muhasebeizleme", provider.Config{APIKey: key, Test: true})
```

| Field | Meaning |
| --- | --- |
| `Username`, `Password` | Web-service credentials. For muhasebeizleme `Password` is the API key (`provider.Config.APIKey` is accepted too) and `Username` is informative. |
| `Preset` | `uyumsoft.Uyumsoft` (default) or `uyumsoft.Muhasebeizleme`. |
| `Test` | Use the preset's test endpoint. |
| `Endpoint` | Override the endpoint (another compatible vendor, a proxy). |
| `Scenario` | Default `InvoiceInfo/Scenario`: empty derives `eInvoice` / `eArchive` from `cbc:ProfileID`; `Automated` lets the integrator decide from the GİB user list. Registry option `scenario`. |
| `NoTimestamp` | Omit `wsu:Timestamp` from the WS-Security header. Registry option `timestamp=false`. |
| `HTTP` | Custom `provider.Doer`. |

## Presets

| | `Uyumsoft` | `Muhasebeizleme` |
| --- | --- | --- |
| Test endpoint | `https://efatura-test.uyumsoft.com.tr/Services/Integration` | `https://test.api.muhasebeizleme.com/Services/BasicIntegration.svc` |
| Live endpoint | `https://efatura.uyumsoft.com.tr/Services/Integration` | `https://api.muhasebeizleme.com/Services/BasicIntegration.svc` |
| SOAP | 1.1 (`BasicHttpBinding`) | 1.1 (`BasicHttpBinding`) |
| Auth | WS-Security `UsernameToken` (PasswordText) + `Timestamp` in the header, per the binding policy | `<userInfo Username=".." Password="API key"/>` as the first child of every operation |
| SOAPAction | `http://tempuri.org/IIntegration/{op}` (`ICustomerServiceBase` for `GetCustomerCreditInfo`) | `http://tempuri.org/IBasicIntegration/{op}` |
| `SetInvoicesTaken` list | `<invoices>` | `<invoiceIds>` |
| `ListInbox` `OnlyNew` | `OnlyNewestInvoices="true"` | `<IsArchived>false</IsArchived>` |
| Outbox `Kind` filter | server side (`Scenario`) | on the returned page |
| Page size | not documented | ≤ 1000 |

Both vendors spell the result flag `IsSucceded`. The WSDLs are in
[`testdata/`](testdata/), and a test checks that every operation the module
calls exists there with the SOAPAction the preset computes.

## Capabilities

Both presets declare the same operations, all documented by both vendors:

| Operation | e-Fatura | e-Arşiv | SOAP operations |
| --- | :-: | :-: | --- |
| Send | ✓ | ✓ | `SendInvoice` |
| Status | ✓ (in/out) | ✓ (out) | `QueryOutboxInvoiceStatus`, `QueryInboxInvoiceStatus` |
| ListInbox | ✓ | | `GetInboxInvoiceList` |
| ListOutbox | ✓ | ✓ | `GetOutboxInvoiceList` |
| Fetch (UBL, PDF, HTML) | ✓ | ✓ | `Get{Inbox,Outbox}Invoice{Data,Pdf,View}` |
| Respond (KABUL / RED) | ✓ | | `SendDocumentResponse` |
| CancelEArchive | | ✓ | `CancelEArchiveInvoice` |
| CheckUser | ✓ | | `IsEInvoiceUser`, `GetUserAliasses` |
| MarkTaken | ✓ | | `SetInvoicesTaken` |
| Credits | ✓ | ✓ | `GetCustomerCreditInfo` |

e-İrsaliye is not claimed: the contract has no despatch operations
(`GetUserAliasses` does report despatch aliases, returned with
`Kind: provider.EDespatch`). Muhasebeizleme also sets `AssignsNumbers`,
`SignsDocuments` and a 5-minute `MinPollInterval`, as its guide documents.

Extra methods: `SendInvoices` (batch, `LocalDocumentId`, e-mail
notification, e-Arşiv delivery type), `CreditDetails`, `Ping`
(`TestConnection`).

### Notes

- **Send** embeds the UBL as the content of `InvoiceInfo/Invoice` (an element
  in the service namespace with `cbc:`/`cac:` children), keeping the root's
  namespace declarations. `ReceiverVKN` / `ReceiverAlias` go to
  `TargetCustomer`, `Email` to `Notification/Mailing`. `InvoiceInfo` has no
  sender-alias field, so `SenderAlias` is ignored.
- **List rows**: `DocumentId` is the ETTN (`DocumentInfo.UUID`), `InvoiceId` is
  the invoice number (`DocumentInfo.ID`). `Profile` / `Type` are derived from
  the `InvoiceTypes` / `InvoiceTipType` enums. With `OnlyNew`, restart from
  the first page after `MarkTaken` instead of following `Page.Next`.
- **Times** without an offset are read and written as Türkiye time (UTC+3),
  including fields named `…Utc`.
- **CancelEArchive** ignores `reason`: `EArchiveCancelInvoiceContext` has no such
  field.

## Status mapping

`InvoiceStatus` values (WSDL enum) map onto `provider.Status`; the raw value
stays in `ProviderCode`, the envelope status becomes `GIBCode` (GİB 1000–1300).

| InvoiceStatus | Status |
| --- | --- |
| `NotPrepared`, `NotSend`, `Draft` | `draft` |
| `Queued` | `queued` |
| `Processing` | `processing` |
| `SentToGib` | `sent` |
| `WaitingForAprovement` | `delivered` (outbox), `received` (inbox) |
| `Approved` | `delivered` (outbox), `received` (inbox); `accepted` for a known TICARIFATURA |
| `Declined`, `Return` | `rejected` |
| `Canceled`, `EArchivedCanceled` | `cancelled` |
| `Error` | `failed` |

List rows carry the profile, so `Approved` becomes `accepted` for a
TICARIFATURA. Status queries return no profile: an outbox `Approved` there is
`delivered`. `StatusResult.ResponseCode` is `RED` / `IADE` for an inbound
`Declined` / `Return`.
For muhasebeizleme, a status text the module does not know falls back to the
documented `StatusCode` scale (0, 10, 100, 200, 300, 1000, 1100, 1200, 1400,
2000).

## Errors

All errors are `*provider.Error`:

- SOAP faults are reclassified by fault code (`InvalidSecurity`,
  `FailedAuthentication` → `auth`; `ActionNotSupported` → `invalid-request`)
  or by the fault text. The muhasebeizleme API-key fault becomes `auth`.
- `IsSucceded="false"`, or `Value="false"` on flag results such as
  `CancelEArchiveInvoice`, is classified from the Turkish `Message` with
  keyword rules in `errors.go`. Example classes: credentials → `auth`,
  kontör → `quota-exceeded`, already sent → `duplicate`, receiver not
  registered or bad alias → `recipient`, schema/date/currency →
  `validation`, cancel window or already answered → `invalid-state`, and
  transient "… alınamadı" → `unavailable` (retryable). `Code` stays empty,
  because the services send no error codes.
- Classes, faults and `InvoiceStatus` rows live in the embedded
  [`errors.csv`](errors.csv) (`uyumsoft.Codes`), which cites its sources.

```go
if errors.Is(err, provider.ErrQuotaExceeded) { … }
```

## Live verification

The muhasebeizleme preset was verified against its test system
(`test.api.muhasebeizleme.com`) on 2026-11-16 with a vendor-issued API key:
credits, outbox and inbox listing (with paging), `Status`, UBL/PDF/HTML
fetch, `CheckUser`, and an end-to-end `Send` of a builder-made TEMELFATURA
that reached `SentToGib`.

The first live send failed the integrator's checks: the integrator signs
documents itself and appends its own `ds:Signature` and `cac:Signature`, so
the builder's `ExtensionContent` placeholder and `cac:Signature` produced
two `ExtensionContent` children (XSD) and two `cac:Signature` elements
(GİB Schematron). For unsigned documents (no `ds:Signature`), `Send` now
removes both before embedding; signed documents are sent unchanged. The
Uyumsoft preset uses the same code path but has not been run live.

## Tests

```bash
go test ./...                      # httptest replay of WSDL-shaped fixtures
```

Live tests run against the vendors' test systems. They only read and never
send, answer or cancel documents:

```bash
UYUMSOFT_USERNAME=… UYUMSOFT_PASSWORD=… go test -tags live -run Live -v ./...
# optional: UYUMSOFT_ENDPOINT, UYUMSOFT_LIVE_VKN,
#           MUHASEBEIZLEME_USERNAME, MUHASEBEIZLEME_API_KEY
```

## Sources

- Uyumsoft Integration WSDL: <https://efatura-test.uyumsoft.com.tr/Services/Integration?singleWsdl> (live: `https://efatura.uyumsoft.com.tr/Services/Integration?singleWsdl`)
- muhasebeizleme BasicIntegration WSDL: <https://api.muhasebeizleme.com/Services/BasicIntegration.svc?singleWsdl>
- Mi API Kılavuzu: <https://api.muhasebeizleme.com/kilavuz/>
