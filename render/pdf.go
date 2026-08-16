package render

import "context"

// PDFRenderer turns HTML into PDF. It must not run XSLT.
type PDFRenderer interface {
	RenderPDF(ctx context.Context, html []byte, opt PDFOptions) ([]byte, error)
}

// PDFOptions controls paper geometry. Zero values mean A4 portrait
// with Chromium's default 0.4 inch margins.
type PDFOptions struct {
	Landscape    bool
	MarginTop    float64
	MarginBottom float64
	MarginLeft   float64
	MarginRight  float64
}

// paperA4WidthInches is ISO 216 A4 width.
const paperA4WidthInches = 8.27

// paperA4HeightInches is ISO 216 A4 height.
const paperA4HeightInches = 11.69

// A4WidthInches returns the A4 portrait width used by PDF adapters.
func A4WidthInches() float64 { return paperA4WidthInches }

// A4HeightInches returns the A4 portrait height used by PDF adapters.
func A4HeightInches() float64 { return paperA4HeightInches }

// ToPDF runs XML → HTML → PDF using the given engines.
func ToPDF(
	ctx context.Context,
	html *HTMLRenderer,
	pdf PDFRenderer,
	xmlBytes []byte,
	opt PDFOptions,
) ([]byte, error) {
	if html == nil {
		return nil, ErrTransform
	}
	if pdf == nil {
		return nil, ErrPDFRender
	}
	page, err := html.Render(ctx, xmlBytes)
	if err != nil {
		return nil, err
	}
	out, err := pdf.RenderPDF(ctx, page, opt)
	if err != nil {
		return nil, err
	}
	return out, nil
}
