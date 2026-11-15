# CLI

`ubltr` is a thin command-line front for the library. It does not reimplement XSD, Schematron, or PDF.

```bash
go install github.com/furkanmeclis/go-ubltr/cmd/ubltr@latest
```

Until the CLI module is tagged, install from a clone:

```bash
git clone https://github.com/furkanmeclis/go-ubltr.git
cd go-ubltr
go install ./cmd/ubltr
```

The CLI is a nested module (`github.com/furkanmeclis/go-ubltr/cmd/ubltr`). It may depend on chromedp for `--pdf`. The core `go get github.com/furkanmeclis/go-ubltr` graph does not.

## Commands

```
ubltr validate [--xsd|--schematron] [--type=efatura] [--format=text|json] FILE...
ubltr inspect [--format=text|json] FILE...
ubltr render [--pdf] [--style FILE] FILE
ubltr generate
```

Exit `0` on success (`validate` means every file is valid). Any other outcome is `1`.

Flags may appear before or after files (`ubltr render invoice.xml --pdf` is valid).

### validate

Default is XSD, then GİB Schematron (`type=efatura`) for Invoice, ApplicationResponse, DespatchAdvice (e-İrsaliye), ReceiptAdvice, and envelopes. CreditNote and the e-Arşiv report are checked against XSD only.

```bash
ubltr validate invoice.xml
ubltr validate --xsd invoice.xml
ubltr validate --schematron --type=earchive invoice.xml
ubltr validate irsaliye.xml zarf.xml
ubltr validate testdata/ubltr-1.2.1/*.xml
ubltr validate --format=json invoice.xml > report.json
```

`--xsd` or `--schematron` restricts to one layer. They are mutually exclusive; omit both to run XSD then Schematron. `--schematron` on a CreditNote or e-Arşiv report fails, because GİB has no rules for them. `--type` is `efatura` (default), `earchive`, or `goruntuleme` and only affects ProfileID-related checks.

Every file is checked. Exit is `1` if any file fails.

Text output (default) writes one line per violation to stderr:

```
invoice.xml:84: schematron PartyIdentificationTCKNVKNCheck: cbc:ID elemanının schemeID ... (/inv:Invoice/cac:AccountingCustomerParty/cac:Party/cac:PartyIdentification[1])
```

That is `path[:line]: layer [severity] [rule]: message (location)`. With several files, valid ones print `path: ok` to stdout.

`--format=json` writes one JSON object to stdout and nothing to stderr (flag errors aside):

```json
{
  "valid": false,
  "files": [
    {
      "path": "invoice.xml",
      "kind": "invoice",
      "valid": false,
      "layer": "schematron",
      "violations": [
        {
          "layer": "schematron",
          "path": "cac:PartyIdentification",
          "ruleId": "PartyIdentificationTCKNVKNCheck",
          "severity": "error",
          "message": "cbc:ID elemanının schemeID niteliği değeri 'TCKN' olması durumunda ...",
          "patternId": "invoice",
          "context": "inv:Invoice/cac:AccountingCustomerParty/cac:Party/cac:PartyIdentification",
          "test": "not(cbc:ID/@schemeID='TCKN') or string-length(cbc:ID)=11",
          "location": "/inv:Invoice/cac:AccountingCustomerParty/cac:Party/cac:PartyIdentification[1]",
          "line": 84,
          "element": "cac:PartyIdentification"
        }
      ]
    }
  ]
}
```

`layer` is the layer that failed (XSD stops before Schematron). A file that cannot be read or parsed has `error` and no `layer`. `violations` is always an array. Violation keys match the library's `ubltr.Violation` JSON tags; empty fields are omitted.

Official 2009 samples in `testdata/` pass XSD and fail current Schematron.

### inspect

Prints the library `Inspect` summary (ID, profile, parties, lines, payable).

```bash
ubltr inspect invoice.xml
ubltr inspect --format=json invoice.xml
```

`--format=json` writes `{"files": [{"path": ..., "summary": {"kind", "id", "uuid", "profile", "type", "currency", "supplier", "customer", "lines", "tax", "payable"}}]}` to stdout. A file that fails has `error` instead of `summary`, and the exit code is `1`.

### render

Writes HTML to stdout. `--pdf` writes PDF to stdout.

```bash
ubltr render invoice.xml > invoice.html
ubltr render --style company.xslt invoice.xml > invoice.html
ubltr render --pdf invoice.xml > invoice.pdf
ubltr render invoice.xml --pdf > invoice.pdf
```

Default stylesheet is `render.StyleMinimal`. `--pdf` needs a Chrome or Chromium binary on `PATH`. If Chrome is missing, stderr names `github.com/furkanmeclis/go-ubltr/render/chromium` and Chrome; it does not panic.

### generate

Maintainer command. From a clone of this repository (any subdirectory):

```bash
ubltr generate
```

That is the same as `make generate`: UBL-TR types, e-Arşiv report types, GİB envelope types, and GİB code lists. It is not useful for an application that only imported the module.

### watch

Polls an integrator and prints events as NDJSON (`ubltr watch --provider NAME ... | jq`). See [Watch](watch.md).

## Tests

```bash
go test ./cmd/ubltr/...
```

`make test` includes that package. PDF is not required for the CLI unit tests.
