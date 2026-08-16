package render

import "context"

// Transformer turns XML into a result tree using an XSLT stylesheet.
// Implementations must default-deny filesystem and network access.
type Transformer interface {
	Transform(ctx context.Context, xml, stylesheet []byte) ([]byte, error)
}
