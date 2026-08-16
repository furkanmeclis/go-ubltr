package render_test

import (
	"context"
	"strings"
	"testing"

	"github.com/furkanmeclis/go-ubltr/render"
	"github.com/furkanmeclis/go-ubltr/render/xslt/helium"
)

func TestToPDFUsesHTMLThenPDF(t *testing.T) {
	t.Parallel()

	htmlR, err := render.NewHTML(helium.New(), render.StyleMinimal())
	if err != nil {
		t.Fatalf("NewHTML: %v", err)
	}
	fake := &stubPDF{out: []byte("%PDF-fake")}
	pdf, err := render.ToPDF(t.Context(), htmlR, fake, readInvoice(t), render.PDFOptions{})
	if err != nil {
		t.Fatalf("ToPDF: %v", err)
	}
	if string(pdf) != "%PDF-fake" {
		t.Fatalf("pdf = %q", pdf)
	}
	if !strings.Contains(string(fake.sawHTML), "GIB20090000000001") {
		t.Fatal("PDF adapter did not receive transformed HTML")
	}
}

type stubPDF struct {
	out     []byte
	sawHTML []byte
}

func (s *stubPDF) RenderPDF(_ context.Context, html []byte, _ render.PDFOptions) ([]byte, error) {
	s.sawHTML = html
	return s.out, nil
}
