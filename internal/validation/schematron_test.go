package validation_test

import (
	"testing"

	"github.com/furkanmeclis/go-ubltr/internal/validation"
)

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
		t.Fatal("expected a reason when schematron is unsupported")
	}
	t.Logf("native schematron unsupported (expected): %s", st.Reason)
}
