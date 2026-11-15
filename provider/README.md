# provider

Vendor-neutral contract for GİB özel entegratör services. Each integrator
is a separate module under `provider/<name>`; this module has no vendor
code and no dependency on the core `go-ubltr` module.

```bash
go get github.com/furkanmeclis/go-ubltr/provider
```

## Pieces

| Package | Purpose |
| --- | --- |
| `provider` | Interfaces (`Sender`, `StatusChecker`, `InboxLister`, …), `Capabilities`, normalized `Status`, `*Error` with `ErrorKind`, GİB envelope code table, registry, retry helper |
| `provider/soap` | Small SOAP 1.1 / 1.2 client (faults, MTOM stripping, WS-Security UsernameToken) |
| `provider/pack` | Zip / base64 / MD5 helpers used around UBL payloads |
| `provider/providertest` | In-memory `Fake` integrator and conformance checks |

## Capabilities

Integrators differ. Check before calling:

```go
p, _ := provider.Open("uyumsoft", provider.Config{Username: u, Password: pw, Test: true})
if provider.Supports(p, provider.EInvoice, provider.OpRespond) {
	_ = p.(provider.Responder).Respond(ctx, provider.Response{UUID: id, Accept: true})
}
```

## Errors

Every integrator error is a `*provider.Error`:

- `Kind` — normalized category, matched with `errors.Is(err, provider.ErrQuotaExceeded)` etc.
- `Code` / `Message` — the integrator's own values, unchanged.
- `GIBCode` — GİB envelope status (1000–1300) when known.
- `Retryable`, `HTTPStatus`, `Raw`.

Integrator code lists are CSV files embedded in each module (`errors.csv`)
and parsed with `ParseCodeTable`. Unknown codes become `KindUnknown` but
keep the raw code and message. GİB envelope codes live in
[`gib_codes.csv`](gib_codes.csv).
