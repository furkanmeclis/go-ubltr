package render

import (
	"context"
	_ "embed"
	"fmt"
)

//go:embed testdata/styles/minimal.xsl
var minimalStylesheet []byte

// Style is an XML-to-HTML stylesheet. It is not a PDF engine.
type Style struct {
	Name       string
	Stylesheet []byte
}

// StyleMinimal is a small MIT stylesheet for tests and examples.
// It is not a GİB viewer template.
func StyleMinimal() Style {
	return Style{Name: "minimal", Stylesheet: minimalStylesheet}
}

// HTMLRenderer runs XML → XSLT → HTML.
type HTMLRenderer struct {
	xf    Transformer
	style Style
}

// NewHTML returns a renderer that applies style through xf.
func NewHTML(xf Transformer, style Style) (*HTMLRenderer, error) {
	if xf == nil {
		return nil, fmt.Errorf("nil transformer")
	}
	if len(style.Stylesheet) == 0 {
		return nil, fmt.Errorf("%w: empty stylesheet", ErrStylesheet)
	}
	return &HTMLRenderer{xf: xf, style: style}, nil
}

// Render transforms xmlBytes with the configured stylesheet.
func (r *HTMLRenderer) Render(ctx context.Context, xmlBytes []byte) ([]byte, error) {
	if r == nil || r.xf == nil {
		return nil, fmt.Errorf("nil html renderer")
	}
	out, err := r.xf.Transform(ctx, xmlBytes, r.style.Stylesheet)
	if err != nil {
		return nil, err
	}
	return out, nil
}
