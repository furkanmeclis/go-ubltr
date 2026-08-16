package chromium_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/furkanmeclis/go-ubltr/render"
	"github.com/furkanmeclis/go-ubltr/render/chromium"
	"github.com/furkanmeclis/go-ubltr/render/xslt/helium"
)

func TestRenderPDFTemelFatura(t *testing.T) {
	xml, err := os.ReadFile(filepath.Join("..", "..", "testdata", "ubltr-1.2.1", "TemelFaturaOrnegi.xml"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	htmlR, err := render.NewHTML(helium.New(), render.StyleMinimal())
	if err != nil {
		t.Fatalf("NewHTML: %v", err)
	}

	pdfR := chromium.New(t.Context())
	defer pdfR.Close()

	pdf, err := render.ToPDF(t.Context(), htmlR, pdfR, xml, render.PDFOptions{})
	if err != nil {
		if isMissingChrome(err) {
			t.Skip(err.Error())
		}
		t.Fatalf("ToPDF: %v", err)
	}
	if !bytes.HasPrefix(pdf, []byte("%PDF")) {
		t.Fatalf("not a pdf (%d bytes)", len(pdf))
	}
	if len(pdf) < 200 {
		t.Fatalf("pdf too small: %d bytes", len(pdf))
	}
	if !bytes.Contains(pdf, []byte("/Type /Page")) && !bytes.Contains(pdf, []byte("/Type/Page")) {
		t.Fatal("pdf has no page object")
	}
}

func TestRenderPDFRejectsWhenClosed(t *testing.T) {
	t.Parallel()

	var r *chromium.Renderer
	_, err := r.RenderPDF(t.Context(), []byte("<html><body>x</body></html>"), render.PDFOptions{})
	if !errors.Is(err, render.ErrPDFRender) {
		t.Fatalf("error = %v, want ErrPDFRender", err)
	}
}

func isMissingChrome(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "executable file not found") ||
		strings.Contains(msg, "chrome not found") ||
		strings.Contains(msg, "google-chrome") && strings.Contains(msg, "not found")
}
