package chromium_test

import (
	"errors"
	"testing"

	"github.com/furkanmeclis/go-ubltr/render"
	"github.com/furkanmeclis/go-ubltr/render/chromium"
)

func TestRenderPDFTemelFatura(t *testing.T) {
	t.Skip("Chrome PDF smoke test is disabled")
}

func TestRenderPDFRejectsWhenClosed(t *testing.T) {
	t.Parallel()

	var r *chromium.Renderer
	_, err := r.RenderPDF(t.Context(), []byte("<html><body>x</body></html>"), render.PDFOptions{})
	if !errors.Is(err, render.ErrPDFRender) {
		t.Fatalf("error = %v, want ErrPDFRender", err)
	}
}
