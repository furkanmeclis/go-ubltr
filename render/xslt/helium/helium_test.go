package helium_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/furkanmeclis/go-ubltr/render"
	"github.com/furkanmeclis/go-ubltr/render/xslt/helium"
)

func TestTransformIgnoresSchemaLocation(t *testing.T) {
	t.Parallel()

	xml, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", "ubltr-1.2.1", "TemelFaturaOrnegi.xml"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	out, err := helium.New().Transform(t.Context(), xml, render.StyleMinimal().Stylesheet)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	if !strings.Contains(string(out), "GIB20090000000001") {
		t.Fatal("expected invoice id in HTML")
	}
}

func TestTransformDoesNotLoadExternalDocument(t *testing.T) {
	t.Parallel()

	xsl := []byte(`<?xml version="1.0"?>
<xsl:stylesheet version="3.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:output method="text"/>
  <xsl:template match="/">
    <xsl:value-of select="doc('/etc/passwd')"/>
  </xsl:template>
</xsl:stylesheet>`)
	_, err := helium.New().Transform(t.Context(), []byte(`<root/>`), xsl)
	if err == nil {
		t.Fatal("expected document() to fail without host file access")
	}
	if errors.Is(err, render.ErrInvalidXML) {
		t.Fatalf("wrong sentinel: %v", err)
	}
}
