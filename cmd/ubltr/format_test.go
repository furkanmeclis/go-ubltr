package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/furkanmeclis/go-ubltr"
)

func decodeValidate(t *testing.T, out []byte) validateReport {
	t.Helper()
	var r validateReport
	if err := json.Unmarshal(out, &r); err != nil {
		t.Fatalf("json: %v\n%s", err, out)
	}
	return r
}

func TestValidateJSONSchematronFailure(t *testing.T) {
	t.Parallel()

	var out, errBuf bytes.Buffer
	code := run([]string{"validate", fixture(t), "--format=json"}, &out, &errBuf)
	if code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
	if errBuf.Len() != 0 {
		t.Fatalf("json mode wrote to stderr: %s", errBuf.String())
	}
	r := decodeValidate(t, out.Bytes())
	if r.Valid || len(r.Files) != 1 {
		t.Fatalf("report = %+v", r)
	}
	f := r.Files[0]
	if f.Kind != ubltr.KindInvoice || f.Layer != ubltr.LayerSchematron || f.Valid {
		t.Fatalf("file = %+v", f)
	}
	found := false
	for _, v := range f.Violations {
		if v.RuleID == "PartyIdentificationTCKNVKNCheck" {
			found = true
			if v.PatternID != "invoice" || v.Location == "" || v.Line == 0 || v.Severity != ubltr.SeverityError {
				t.Fatalf("violation = %+v", v)
			}
		}
	}
	if !found {
		t.Fatalf("no TCKN violation in %+v", f.Violations)
	}
	if !strings.Contains(out.String(), `"ruleId"`) || !strings.Contains(out.String(), `"location"`) {
		t.Fatalf("expected camelCase keys:\n%s", out.String())
	}
}

func TestValidateJSONValid(t *testing.T) {
	t.Parallel()

	var out, errBuf bytes.Buffer
	code := run([]string{"validate", "--xsd", "--format", "json", fixture(t)}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("exit %d, stderr=%s", code, errBuf.String())
	}
	r := decodeValidate(t, out.Bytes())
	if !r.Valid || !r.Files[0].Valid || r.Files[0].Violations == nil {
		t.Fatalf("report = %+v", r)
	}
}

func TestValidateJSONReadError(t *testing.T) {
	t.Parallel()

	var out, errBuf bytes.Buffer
	missing := filepath.Join(t.TempDir(), "missing.xml")
	code := run([]string{"validate", "--format=json", missing}, &out, &errBuf)
	if code != 1 {
		t.Fatalf("exit %d", code)
	}
	r := decodeValidate(t, out.Bytes())
	if r.Files[0].Error == "" || r.Files[0].Layer != "" {
		t.Fatalf("file = %+v", r.Files[0])
	}
}

func TestValidateBadFormat(t *testing.T) {
	t.Parallel()

	var out, errBuf bytes.Buffer
	if code := run([]string{"validate", "--format=xml", fixture(t)}, &out, &errBuf); code != 1 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(errBuf.String(), "--format") {
		t.Fatalf("stderr = %q", errBuf.String())
	}
}

func TestValidateTextShowsRuleAndLine(t *testing.T) {
	t.Parallel()

	var out, errBuf bytes.Buffer
	path := fixture(t)
	if code := run([]string{"validate", path}, &out, &errBuf); code != 1 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(errBuf.String(), path+":84: schematron PartyIdentificationTCKNVKNCheck:") {
		t.Fatalf("stderr = %s", errBuf.String())
	}
}

func TestValidateSchematronDespatchAdvice(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "irsaliye.xml")
	doc := `<?xml version="1.0" encoding="UTF-8"?>
<DespatchAdvice xmlns="urn:oasis:names:specification:ubl:schema:xsd:DespatchAdvice-2"/>
`
	if err := os.WriteFile(path, []byte(doc), 0o600); err != nil {
		t.Fatal(err)
	}
	var out, errBuf bytes.Buffer
	if code := run([]string{"validate", "--schematron", "--format=json", path}, &out, &errBuf); code != 1 {
		t.Fatalf("exit %d", code)
	}
	r := decodeValidate(t, out.Bytes())
	f := r.Files[0]
	if f.Kind != ubltr.KindDespatchAdvice || f.Layer != ubltr.LayerSchematron {
		t.Fatalf("file = %+v", f)
	}
	for _, v := range f.Violations {
		if v.RuleID == "DespatchDateCheck" && v.PatternID == "despatchadvice" {
			return
		}
	}
	t.Fatalf("DespatchDateCheck missing: %+v", f.Violations)
}

func TestInspectJSON(t *testing.T) {
	t.Parallel()

	var out, errBuf bytes.Buffer
	code := run([]string{"inspect", "--format=json", fixture(t)}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("exit %d, stderr=%s", code, errBuf.String())
	}
	var r inspectReport
	if err := json.Unmarshal(out.Bytes(), &r); err != nil {
		t.Fatalf("json: %v\n%s", err, out.String())
	}
	s := r.Files[0].Summary
	if s == nil || s.Kind != ubltr.KindInvoice || s.ID != "GIB20090000000001" || s.Payable != "17.88" {
		t.Fatalf("summary = %+v", s)
	}
	if !strings.Contains(out.String(), `"payable": "17.88"`) {
		t.Fatalf("json keys:\n%s", out.String())
	}
}

func TestInspectJSONError(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "foo.xml")
	if err := os.WriteFile(path, []byte(`<Foo/>`), 0o600); err != nil {
		t.Fatal(err)
	}
	var out, errBuf bytes.Buffer
	if code := run([]string{"inspect", path, "--format=json"}, &out, &errBuf); code != 1 {
		t.Fatalf("exit %d", code)
	}
	var r inspectReport
	if err := json.Unmarshal(out.Bytes(), &r); err != nil {
		t.Fatalf("json: %v", err)
	}
	if r.Files[0].Error == "" || r.Files[0].Summary != nil {
		t.Fatalf("file = %+v", r.Files[0])
	}
}
