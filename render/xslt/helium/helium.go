// Package helium implements [render.Transformer] with helium xslt3.
//
// Filesystem and network access stay disabled. Callers who need xsl:include
// or document() must use a different adapter.
package helium

import (
	"context"
	"fmt"
	"io"
	"io/fs"

	"github.com/lestrrat-go/helium"
	"github.com/lestrrat-go/helium/xslt3"

	"github.com/furkanmeclis/go-ubltr/render"
)

// Transformer compiles stylesheets and runs them in memory.
type Transformer struct{}

// New returns a helium XSLT transformer.
func New() *Transformer {
	return &Transformer{}
}

// closedResolver never returns bytes. It is installed so GİB documents that
// carry xsi:schemaLocation are treated as a fetch miss (skipped under lax
// validation) instead of a fatal "no resolver" policy error. xsl:include and
// document() still cannot read the host.
type closedResolver struct{}

func (closedResolver) ResolveURI(string) (io.ReadCloser, error) {
	return nil, fs.ErrNotExist
}

// Transform applies stylesheet to xml. Host filesystem and network stay closed.
func (t *Transformer) Transform(ctx context.Context, xml, stylesheet []byte) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if t == nil {
		return nil, fmt.Errorf("%w: nil transformer", render.ErrTransform)
	}

	p := helium.NewParser()
	styleDoc, err := p.Parse(ctx, stylesheet)
	if err != nil {
		return nil, fmt.Errorf("%w: parse stylesheet: %w", render.ErrStylesheet, err)
	}
	ss, err := xslt3.CompileStylesheet(ctx, styleDoc)
	if err != nil {
		return nil, fmt.Errorf("%w: compile stylesheet: %w", render.ErrStylesheet, err)
	}

	src, err := p.Parse(ctx, xml)
	if err != nil {
		return nil, fmt.Errorf("%w: parse xml: %w", render.ErrInvalidXML, err)
	}

	out, err := ss.Transform(src).URIResolver(closedResolver{}).Serialize(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", render.ErrTransform, err)
	}
	return []byte(out), nil
}
