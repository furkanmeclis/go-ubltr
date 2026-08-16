package main

import (
	"context"
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
	docType := fs.String("type", ubltr.TypeEFatura, "Schematron type: efatura or earchive")
	fs.Usage = func() {
		fmt.Fprint(stderr, `Usage: ubltr validate [options] FILE...

  --xsd           XSD only
  --schematron    Schematron only
  --type=efatura  Invoice Schematron type (efatura or earchive)

Default is XSD then Invoice Schematron. Non-Invoice documents skip Schematron.
Exit 0 if every file is valid; otherwise 1. All files are checked.
`)
	}
	if err := fs.Parse(flagsFirst(args, "type")); err != nil {
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
	if typ != ubltr.TypeEFatura && typ != ubltr.TypeEArchive {
		fmt.Fprintf(stderr, "ubltr validate: --type must be %s or %s\n", ubltr.TypeEFatura, ubltr.TypeEArchive)
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
	for _, path := range files {
		if err := validateFile(ctx, path, mode, typ); err != nil {
			failed = true
			printValidateErr(stderr, path, err)
			continue
		}
		if len(files) > 1 {
			fmt.Fprintf(stdout, "%s: ok\n", path)
		}
	}
	if failed {
		return 1
	}
	return 0
}

func validateFile(ctx context.Context, path, mode, docType string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	kind, err := ubltr.Detect(data)
	if err != nil {
		return err
	}
	runXSD := mode == "all" || mode == "xsd"
	runSch := mode == "all" || mode == "schematron"
	if runXSD {
		if err := ubltr.ValidateXSD(ctx, data); err != nil {
			return err
		}
	}
	if runSch && kind == ubltr.KindInvoice {
		if err := ubltr.ValidateSchematronType(ctx, data, docType); err != nil {
			return err
		}
	}
	if runSch && mode == "schematron" && kind != ubltr.KindInvoice {
		return fmt.Errorf("schematron applies only to Invoice documents")
	}
	return nil
}

func printValidateErr(w io.Writer, path string, err error) {
	var xe *ubltr.XSDValidationError
	var se *ubltr.SchematronValidationError
	switch {
	case errors.As(err, &xe):
		for _, msg := range xe.Messages {
			fmt.Fprintf(w, "%s: %s\n", path, msg)
		}
	case errors.As(err, &se):
		for _, msg := range se.Messages {
			fmt.Fprintf(w, "%s: %s\n", path, msg)
		}
	default:
		fmt.Fprintf(w, "%s: %v\n", path, err)
	}
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
