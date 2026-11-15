package codegen

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// TestGenerateEnvelopeMatchesCheckedIn regenerates schema/envelope into a
// temporary directory and compares it with the committed files.
func TestGenerateEnvelopeMatchesCheckedIn(t *testing.T) {
	t.Parallel()

	root := filepath.Join("..", "..")
	out := t.TempDir()
	entry := filepath.Join(root, "schemas", "ubltr-1.2.1", "envelope", "PackageProxy_1_2.xsd")
	if err := GenerateEnvelope(entry, out); err != nil {
		t.Fatalf("GenerateEnvelope: %v", err)
	}
	for _, name := range []string{"types.go", "generated.go", "doc.go"} {
		got, err := os.ReadFile(filepath.Join(out, name))
		if err != nil {
			t.Fatal(err)
		}
		want, err := os.ReadFile(filepath.Join(root, "schema", "envelope", name))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("%s differs from schema/envelope; run make generate", name)
		}
	}
}
