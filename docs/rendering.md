# Rendering

Style (XML → HTML) and PDF engine (HTML → PDF) are different modules. Core does not import them.

Primary input is `[]byte` XML. Generated Invoice types are optional: marshal first if you have a struct.

## HTML

```go
import (
	"github.com/furkanmeclis/go-ubltr/render"
	heliumxslt "github.com/furkanmeclis/go-ubltr/render/xslt/helium"
)

htmlR, err := render.NewHTML(heliumxslt.New(), render.StyleMinimal())
html, err := htmlR.Render(ctx, xmlBytes)
```

Your stylesheet:

```go
xsl, err := os.ReadFile("invoice.xslt") // .xsl and .xslt are the same format
style := render.Style{Name: "company", Stylesheet: xsl}
htmlR, err := render.NewHTML(heliumxslt.New(), style)
```

`StyleMinimal` is a small MIT sheet for tests. GİB `general.xslt` is in [`examples/render/general/`](../examples/render/general/) for local trials, not as a core `go get` asset.

Default transformer is helium xslt3 (pure Go):

- `xsi:schemaLocation` on GİB XML is ignored (no XSD fetch).
- `document()` and `xsl:include` cannot read the host filesystem or network.

Sentinels: `render.ErrInvalidXML`, `render.ErrStylesheet`, `render.ErrTransform`. Use `errors.Is`.

## PDF

```go
import "github.com/furkanmeclis/go-ubltr/render/chromium"

pdfR := chromium.New(ctx)
defer pdfR.Close()
pdf, err := render.ToPDF(ctx, htmlR, pdfR, xmlBytes, render.PDFOptions{})
```

`ToPDF` is XML → HTML → PDF. `PDFRenderer.RenderPDF` is HTML → PDF only.

Zero `PDFOptions` means A4 portrait. Set `Landscape: true` for landscape. Margins are inches; leave them zero for Chromium defaults (~0.4 in).

HTTP, HTTPS, and `file://` loads are blocked during print. Put images in the HTML as `data:` URLs (GİB `general.xslt` already embeds the e-Fatura logo that way).

`render.ErrPDFRender` wraps Chrome errors. Tests skip when Chrome is missing.

## Examples

| Program | What it does |
| --- | --- |
| `examples/render/html` | `StyleMinimal` → HTML |
| `examples/render/custom-style` | file XSLT → HTML |
| `examples/render/general` | GİB `general.xslt` → HTML |
| `examples/render/pdf` | HTML → PDF (`-xsl` optional) |

```bash
cd examples/render/general
go run . -in ../../../testdata/ubltr-1.2.1/TemelFaturaOrnegi.xml

cd examples/render/pdf
go run . -in ../../../testdata/ubltr-1.2.1/TemelFaturaOrnegi.xml \
  -xsl ../general/general.xslt -out ../general/TemelFatura.pdf
```

Command-line HTML/PDF: [cli.md](cli.md) (`ubltr render`, `ubltr render --pdf`).
