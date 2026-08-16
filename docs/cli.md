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
ubltr validate [options] FILE...
ubltr inspect FILE...
ubltr render [--pdf] [--style FILE] FILE
ubltr generate
```

Exit `0` on success (`validate` means every file is valid). Any other outcome is `1`.

Flags may appear before or after files (`ubltr render invoice.xml --pdf` is valid).

### validate

Default is XSD, then Invoice Schematron (`type=efatura`). CreditNote and other kinds skip Schematron.

```bash
ubltr validate invoice.xml
ubltr validate --xsd invoice.xml
ubltr validate --schematron --type=earchive invoice.xml
ubltr validate testdata/ubltr-1.2.1/*.xml
```

`--xsd` or `--schematron` restricts to one layer. They are mutually exclusive; omit both to run XSD then Invoice Schematron. `--type` is `efatura` (default) or `earchive` and only applies to Invoice Schematron.

Every file is checked. Failures go to stderr, prefixed with the path. Exit is `1` if any file fails.

Official 2009 samples in `testdata/` pass XSD and fail current Schematron.

### inspect

Prints the library `Inspect` summary (ID, profile, parties, lines, payable).

```bash
ubltr inspect invoice.xml
```

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

That is the same as `make generate`: UBL-TR types, e-Arşiv report types, and GİB code lists. It is not useful for an application that only imported the module.

## Tests

```bash
go test ./cmd/ubltr/...
```

`make test` includes that package. PDF is not required for the CLI unit tests.
