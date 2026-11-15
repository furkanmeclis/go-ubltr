package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/furkanmeclis/go-ubltr"
)

func cmdValidate(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("validate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	xsdOnly := fs.Bool("xsd", false, "XSD only")
	schOnly := fs.Bool("schematron", false, "Schematron only")
	docType := fs.String("type", ubltr.TypeEFatura, "Schematron type: efatura, earchive, or goruntuleme")
	format := fs.String("format", formatText, "output format: text or json")
	fs.Usage = func() {
		fmt.Fprint(stderr, `Usage: ubltr validate [options] FILE...

  --xsd           XSD only
  --schematron    Schematron only
  --type=efatura  Schematron type (efatura, earchive, or goruntuleme)
  --format=text   Output format: text (stderr) or json (stdout)

Default is XSD then GİB Schematron. Schematron runs for Invoice,
ApplicationResponse, DespatchAdvice, ReceiptAdvice, and envelopes;
CreditNote and the e-Arşiv report are checked against XSD only.
Exit 0 if every file is valid; otherwise 1. All files are checked.
`)
	}
	if err := fs.Parse(flagsFirst(args, "type", "format")); err != nil {
		return 1
	}
	if err := checkFormat(*format); err != nil {
		fmt.Fprintf(stderr, "ubltr validate: %v\n", err)
		return 1
	}
	files, err := expandFiles(fs.Args())
	if err != nil {
		fmt.Fprintf(stderr, "ubltr validate: %v\n", err)
		return 1
	}
	if len(files) == 0 {
		fs.Usage()
		return 1
	}
	if *xsdOnly && *schOnly {
		fmt.Fprintln(stderr, "ubltr validate: use only one of --xsd or --schematron")
		return 1
	}
	typ := strings.TrimSpace(*docType)
	if typ != ubltr.TypeEFatura && typ != ubltr.TypeEArchive && typ != ubltr.TypeGoruntuleme {
		fmt.Fprintf(stderr, "ubltr validate: --type must be %s, %s, or %s\n",
			ubltr.TypeEFatura, ubltr.TypeEArchive, ubltr.TypeGoruntuleme)
		return 1
	}

	mode := "all"
	switch {
	case *xsdOnly && !*schOnly:
		mode = "xsd"
	case *schOnly && !*xsdOnly:
		mode = "schematron"
	}

	ctx := context.Background()
	failed := false
	report := validateReport{Valid: true, Files: make([]validateFileReport, 0, len(files))}
	for _, path := range files {
		kind, err := validateFile(ctx, path, mode, typ)
		report.Files = append(report.Files, newValidateFileReport(path, kind, err))
		if err != nil {
			failed = true
			report.Valid = false
			if *format == formatText {
				printValidateErr(stderr, path, err)
			}
			continue
		}
		if *format == formatText && len(files) > 1 {
			fmt.Fprintf(stdout, "%s: ok\n", path)
		}
	}
	if *format == formatJSON {
		if err := writeJSON(stdout, report); err != nil {
			fmt.Fprintf(stderr, "ubltr validate: %v\n", err)
			return 1
		}
	}
	if failed {
		return 1
	}
	return 0
}

// validateReport is the --format=json output of ubltr validate.
type validateReport struct {
	Valid bool                 `json:"valid"`
	Files []validateFileReport `json:"files"`
}

type validateFileReport struct {
	Path  string     `json:"path"`
	Kind  ubltr.Kind `json:"kind,omitempty"`
	Valid bool       `json:"valid"`
	// Layer is the layer that failed: xsd or schematron. Empty when valid or
	// when the file could not be read or parsed.
	Layer      string            `json:"layer,omitempty"`
	Error      string            `json:"error,omitempty"`
	Violations []ubltr.Violation `json:"violations"`
}

func newValidateFileReport(path string, kind ubltr.Kind, err error) validateFileReport {
	r := validateFileReport{Path: path, Kind: kind, Valid: err == nil, Violations: []ubltr.Violation{}}
	if err == nil {
		return r
	}
	var xe *ubltr.XSDValidationError
	var se *ubltr.SchematronValidationError
	switch {
	case errors.As(err, &xe):
		r.Layer = ubltr.LayerXSD
	case errors.As(err, &se):
		r.Layer = ubltr.LayerSchematron
	default:
		r.Error = err.Error()
	}
	if vs := ubltr.Violations(err); vs != nil {
		r.Violations = vs
	}
	return r
}

func validateFile(ctx context.Context, path, mode, docType string) (ubltr.Kind, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	kind, err := ubltr.Detect(data)
	if err != nil {
		return "", err
	}
	runXSD := mode == "all" || mode == "xsd"
	runSch := mode == "all" || mode == "schematron"
	if runXSD {
		if err := ubltr.ValidateXSD(ctx, data); err != nil {
			return kind, err
		}
	}
	if runSch && ubltr.HasSchematron(kind) {
		if err := ubltr.ValidateSchematronType(ctx, data, docType); err != nil {
			return kind, err
		}
	}
	if runSch && mode == "schematron" && !ubltr.HasSchematron(kind) {
		return kind, fmt.Errorf("GİB Schematron has no rules for %s documents", kind)
	}
	return kind, nil
}

func printValidateErr(w io.Writer, path string, err error) {
	var xe *ubltr.XSDValidationError
	var se *ubltr.SchematronValidationError
	switch {
	case errors.As(err, &xe):
		for _, v := range xe.Violations {
			fmt.Fprintln(w, textViolation(path, v))
		}
	case errors.As(err, &se):
		for _, v := range se.Violations {
			fmt.Fprintln(w, textViolation(path, v))
		}
	default:
		fmt.Fprintf(w, "%s: %v\n", path, err)
	}
}

// textViolation formats one violation for the default text output:
// path[:line]: layer [severity] [RuleID]: message (location).
func textViolation(path string, v ubltr.Violation) string {
	var b strings.Builder
	b.WriteString(path)
	if v.Line > 0 {
		fmt.Fprintf(&b, ":%d", v.Line)
	}
	b.WriteString(": " + v.Layer)
	if v.Severity != "" && v.Severity != ubltr.SeverityError {
		b.WriteString(" " + v.Severity)
	}
	if v.RuleID != "" {
		b.WriteString(" " + v.RuleID)
	}
	b.WriteString(": " + v.Message)
	switch {
	case v.Location != "":
		b.WriteString(" (" + v.Location + ")")
	case v.Path != "":
		b.WriteString(" (" + v.Path + ")")
	}
	return b.String()
}

const (
	formatText = "text"
	formatJSON = "json"
)

func checkFormat(f string) error {
	if f != formatText && f != formatJSON {
		return fmt.Errorf("--format must be %s or %s", formatText, formatJSON)
	}
	return nil
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

func expandFiles(args []string) ([]string, error) {
	out := make([]string, 0, len(args))
	for _, a := range args {
		if strings.ContainsAny(a, "*?[") {
			matches, err := filepath.Glob(a)
			if err != nil {
				return nil, fmt.Errorf("glob %s: %w", a, err)
			}
			if len(matches) == 0 {
				return nil, fmt.Errorf("no files match %s", a)
			}
			out = append(out, matches...)
			continue
		}
		out = append(out, a)
	}
	return out, nil
}
