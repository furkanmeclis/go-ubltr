package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixture(t *testing.T) string {
	t.Helper()
	p := filepath.Join("..", "..", "testdata", "ubltr-1.2.1", "TemelFaturaOrnegi.xml")
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("fixture: %v", err)
	}
	return p
}

func TestHelp(t *testing.T) {
	t.Parallel()

	var out, errBuf bytes.Buffer
	if code := run([]string{"help"}, &out, &errBuf); code != 0 {
		t.Fatalf("exit %d, stderr=%s", code, errBuf.String())
	}
	if !strings.Contains(out.String(), "validate") {
		t.Fatalf("help = %q", out.String())
	}
}

func TestUnknownCommand(t *testing.T) {
	t.Parallel()

	var out, errBuf bytes.Buffer
	if code := run([]string{"nope"}, &out, &errBuf); code == 0 {
		t.Fatal("expected non-zero")
	}
	if !strings.Contains(errBuf.String(), "unknown command") {
		t.Fatalf("stderr = %q", errBuf.String())
	}
}

func TestValidateXSDOnly(t *testing.T) {
	t.Parallel()

	var out, errBuf bytes.Buffer
	code := run([]string{"validate", "-xsd", fixture(t)}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("exit %d, stderr=%s", code, errBuf.String())
	}
}

func TestValidateXSDFlagAfterFile(t *testing.T) {
	t.Parallel()

	var out, errBuf bytes.Buffer
	code := run([]string{"validate", fixture(t), "--xsd"}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("exit %d, stderr=%s", code, errBuf.String())
	}
}

func TestValidateGlob(t *testing.T) {
	t.Parallel()

	pattern := filepath.Join("..", "..", "testdata", "ubltr-1.2.1", "TemelFatura*.xml")
	var out, errBuf bytes.Buffer
	code := run([]string{"validate", "--xsd", pattern}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("exit %d, stderr=%s", code, errBuf.String())
	}
}

func TestValidateBothLayerFlags(t *testing.T) {
	t.Parallel()

	var out, errBuf bytes.Buffer
	code := run([]string{"validate", "--xsd", "--schematron", fixture(t)}, &out, &errBuf)
	if code == 0 {
		t.Fatal("expected non-zero when both --xsd and --schematron are set")
	}
	if !strings.Contains(errBuf.String(), "--xsd") {
		t.Fatalf("stderr = %q", errBuf.String())
	}
}

func TestValidateSchematronFailsFixture(t *testing.T) {
	t.Parallel()

	var out, errBuf bytes.Buffer
	code := run([]string{"validate", fixture(t)}, &out, &errBuf)
	if code == 0 {
		t.Fatal("expected schematron failure on 2009 fixture")
	}
	if errBuf.Len() == 0 {
		t.Fatal("expected stderr diagnostics")
	}
}

func TestInspect(t *testing.T) {
	t.Parallel()

	var out, errBuf bytes.Buffer
	code := run([]string{"inspect", fixture(t)}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("exit %d, stderr=%s", code, errBuf.String())
	}
	got := out.String()
	for _, want := range []string{"Invoice", "GIB20090000000001", "TEMELFATURA", "17.88"} {
		if !strings.Contains(got, want) {
			t.Fatalf("inspect missing %q:\n%s", want, got)
		}
	}
}

func TestRenderHTML(t *testing.T) {
	t.Parallel()

	var out, errBuf bytes.Buffer
	code := run([]string{"render", fixture(t)}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("exit %d, stderr=%s", code, errBuf.String())
	}
	if !strings.Contains(strings.ToLower(out.String()), "html") {
		t.Fatalf("expected html, got %q", out.String()[:min(200, out.Len())])
	}
}

func TestRenderPDFFlagAfterFile(t *testing.T) {
	var out, errBuf bytes.Buffer
	code := run([]string{"render", fixture(t), "--pdf"}, &out, &errBuf)
	if code == 0 {
		if !bytes.HasPrefix(out.Bytes(), []byte("%PDF")) {
			t.Fatalf("expected %%PDF, got %q", out.String()[:min(32, out.Len())])
		}
		return
	}
	msg := errBuf.String()
	if !strings.Contains(msg, "github.com/furkanmeclis/go-ubltr/render/chromium") {
		t.Fatalf("stderr missing chromium module: %q", msg)
	}
	if !strings.Contains(msg, "Chrome") && !strings.Contains(msg, "Chromium") {
		t.Fatalf("stderr missing Chrome: %q", msg)
	}
}

func TestFindRepoRoot(t *testing.T) {
	t.Parallel()

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root, err := findRepoRoot(cwd)
	if err != nil {
		t.Fatalf("findRepoRoot: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "cmd", "ubltr-gen", "main.go")); err != nil {
		t.Fatalf("root %s: %v", root, err)
	}
}

func TestGenerateNotCheckout(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	_, err := findRepoRoot(dir)
	if err == nil {
		t.Fatal("expected error")
	}
}
