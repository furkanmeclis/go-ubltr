package render_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/furkanmeclis/go-ubltr/render"
	"github.com/furkanmeclis/go-ubltr/render/xslt/helium"
)

func TestHTMLRendererTemelFatura(t *testing.T) {
	t.Parallel()

	xml := readInvoice(t)
	r, err := render.NewHTML(helium.New(), render.StyleMinimal())
	if err != nil {
		t.Fatalf("NewHTML: %v", err)
	}
	html, err := r.Render(t.Context(), xml)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	got := string(html)
	for _, want := range []string{
		"<html",
		"<table",
		"GIB20090000000001",
		"TEMELFATURA",
		"AAA Anonim Şirketi",
		"İstanbul",
		"Türkiye",
		"Elektrik Tüketim Bedeli",
		"ı İş Ş ğ Ğ ü Ü ö Ö ç Ç",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("HTML missing %q", want)
		}
	}
}

func TestHTMLRendererRejectsMalformedXML(t *testing.T) {
	t.Parallel()

	r, err := render.NewHTML(helium.New(), render.StyleMinimal())
	if err != nil {
		t.Fatalf("NewHTML: %v", err)
	}
	_, err = r.Render(t.Context(), []byte(`<not-xml`))
	if !errors.Is(err, render.ErrInvalidXML) {
		t.Fatalf("error = %v, want ErrInvalidXML", err)
	}
}

func TestHTMLRendererRejectsBrokenStylesheet(t *testing.T) {
	t.Parallel()

	r, err := render.NewHTML(helium.New(), render.Style{
		Name:       "broken",
		Stylesheet: []byte(`<xsl:stylesheet xmlns:xsl="http://www.w3.org/1999/XSL/Transform"/>`),
	})
	if err != nil {
		t.Fatalf("NewHTML: %v", err)
	}
	_, err = r.Render(t.Context(), readInvoice(t))
	if err == nil {
		t.Fatal("expected stylesheet error")
	}
	if !errors.Is(err, render.ErrStylesheet) && !errors.Is(err, render.ErrTransform) {
		t.Fatalf("error = %v, want ErrStylesheet or ErrTransform", err)
	}
}

func TestNewHTMLRejectsEmptyStyle(t *testing.T) {
	t.Parallel()

	_, err := render.NewHTML(helium.New(), render.Style{Name: "empty"})
	if !errors.Is(err, render.ErrStylesheet) {
		t.Fatalf("error = %v, want ErrStylesheet", err)
	}
}

func readInvoice(t *testing.T) []byte {
	t.Helper()
	path := filepath.Join("..", "testdata", "ubltr-1.2.1", "TemelFaturaOrnegi.xml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return data
}
