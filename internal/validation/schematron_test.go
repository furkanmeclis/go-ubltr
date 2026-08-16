package validation_test

import (
	"strings"
	"testing"

	"github.com/furkanmeclis/go-ubltr/internal/validation"
)

func TestSchematronCompilesAndRuns(t *testing.T) {
	t.Parallel()

	data := readTestdata(t, "TemelFaturaOrnegi.xml")
	result, err := validation.Schematron(t.Context(), data, validation.TypeEFatura)
	if err != nil {
		t.Fatalf("schematron: %v", err)
	}
	if result.Valid() {
		t.Fatal("2009 GİB sample should fail current schematron (TCKN length)")
	}
	foundTCKN := false
	for _, v := range result.Violations {
		t.Log(v.String())
		if strings.Contains(v.Message, "11 haneli") {
			foundTCKN = true
		}
	}
	if !foundTCKN {
		t.Fatal("expected TCKN length violation on AccountingCustomerParty")
	}
}

func TestSchematronRejectsUnknownProfile(t *testing.T) {
	t.Parallel()

	data := readTestdata(t, "TemelFaturaOrnegi.xml")
	xml := strings.Replace(string(data), ">TEMELFATURA<", ">NOTAPROFILE<", 1)
	result, err := validation.Schematron(t.Context(), []byte(xml), validation.TypeEFatura)
	if err != nil {
		t.Fatalf("schematron: %v", err)
	}
	if result.Valid() {
		t.Fatal("expected profile id violation")
	}
	found := false
	for _, v := range result.Violations {
		if strings.Contains(v.Message, "ProfileID") || v.RuleID == "ProfileIDCheck" {
			found = true
			break
		}
	}
	if !found {
		for _, v := range result.Violations {
			t.Log(v.String())
		}
		t.Fatal("expected a ProfileID violation")
	}
}

func TestSchematronEArchiveRejectsTemelFatura(t *testing.T) {
	t.Parallel()

	data := readTestdata(t, "TemelFaturaOrnegi.xml")
	result, err := validation.Schematron(t.Context(), data, validation.TypeEArchive)
	if err != nil {
		t.Fatalf("schematron: %v", err)
	}
	if result.Valid() {
		t.Fatal("TEMELFATURA should fail e-archive schematron")
	}
}

func TestSchematronRejectsMalformedXML(t *testing.T) {
	t.Parallel()

	_, err := validation.Schematron(t.Context(), []byte(`<not-xml`), "")
	if err == nil {
		t.Fatal("expected parse error")
	}
}

func TestProbeSchematronDocumentsISOGap(t *testing.T) {
	t.Parallel()

	st, err := validation.ProbeSchematron(t.Context())
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if st.Supported {
		t.Log("helium compiled GİB Main Schematron natively")
		return
	}
	if st.Reason == "" {
		t.Fatal("expected a reason when native compile is unsupported")
	}
}
