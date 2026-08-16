package validation_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/furkanmeclis/go-ubltr/internal/validation"
)

func TestXSD(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		xml    []byte
		wantOK bool
	}{
		{
			name:   "temel fatura fixture",
			xml:    readTestdata(t, "TemelFaturaOrnegi.xml"),
			wantOK: true,
		},
		{
			name:   "iade faturasi fixture",
			xml:    readTestdata(t, "IadeFaturasiOrnegi.xml"),
			wantOK: true,
		},
		{
			name: "empty credit note",
			xml: []byte(`<?xml version="1.0" encoding="UTF-8"?>
<CreditNote xmlns="urn:oasis:names:specification:ubl:schema:xsd:CreditNote-2"/>
`),
			wantOK: false,
		},
		{
			name: "empty e-archive report",
			xml: []byte(`<?xml version="1.0" encoding="UTF-8"?>
<eArsivRaporu xmlns="http://earsiv.efatura.gov.tr"/>
`),
			wantOK: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result, err := validation.XSD(t.Context(), tc.xml)
			if err != nil {
				t.Fatalf("xsd compile/validate: %v", err)
			}
			if got := result.Valid(); got != tc.wantOK {
				for _, v := range result.Violations {
					t.Log(v.String())
				}
				t.Fatalf("Valid() = %v, want %v (%d violations)", got, tc.wantOK, len(result.Violations))
			}
		})
	}
}

func TestXSDRejectsMalformedXML(t *testing.T) {
	t.Parallel()

	_, err := validation.XSD(t.Context(), []byte(`<not-xml`))
	if err == nil {
		t.Fatal("expected parse error for malformed xml")
	}
}

func readTestdata(t *testing.T, name string) []byte {
	t.Helper()
	path := filepath.Join("..", "..", "testdata", "ubltr-1.2.1", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return data
}
