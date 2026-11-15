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
