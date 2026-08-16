// Package validation implements XSD and Schematron checks against GİB UBL-TR schemas.
package validation

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"sync"

	"github.com/lestrrat-go/helium"
	"github.com/lestrrat-go/helium/xsd"

	"github.com/furkanmeclis/go-ubltr/schemas"
)

const (
	invoiceXSDRel = "ubltr-1.2.1/xsdrt/maindoc/UBL-Invoice-2.1.xsd"
	invoiceXSDURI = "mem:/" + invoiceXSDRel
	memPrefix     = "mem:/"
)

// Violation is a single schema or business-rule failure.
type Violation struct {
	Layer    string // "xsd" or "schematron"
	Path     string
	RuleID   string
	Severity string
	Message  string
}

func (v Violation) String() string {
	if v.Path != "" {
		return fmt.Sprintf("%s: %s (%s)", v.Layer, v.Message, v.Path)
	}
	return fmt.Sprintf("%s: %s", v.Layer, v.Message)
}

// Result is the outcome of one or more validation layers.
type Result struct {
	Violations []Violation
}

// Valid reports whether validation produced no violations.
func (r Result) Valid() bool {
	return len(r.Violations) == 0
}

// uriFS serves embed paths when helium asks for mem:/… schema locations.
// Open is unsupported so helium uses ReadFile for names that are not fs.ValidPath.
type uriFS struct {
	inner fs.FS
}

func (uriFS) Open(string) (fs.File, error) {
	return nil, fs.ErrNotExist
}

func (f uriFS) ReadFile(name string) ([]byte, error) {
	rel := strings.TrimPrefix(name, memPrefix)
	return fs.ReadFile(f.inner, rel)
}

var compileInvoice = sync.OnceValues(func() (*xsd.Schema, error) {
	ctx := context.Background()
	schemaData, err := fs.ReadFile(schemas.FS, invoiceXSDRel)
	if err != nil {
		return nil, fmt.Errorf("read invoice xsd: %w", err)
	}

	p := helium.NewParser().BaseURI(invoiceXSDURI)
	schemaDoc, err := p.Parse(ctx, schemaData)
	if err != nil {
		return nil, fmt.Errorf("parse invoice xsd: %w", err)
	}

	schema, err := xsd.NewCompiler().
		FS(uriFS{inner: schemas.FS}).
		BaseDir(invoiceXSDURI).
		Label(invoiceXSDURI).
		Parser(p).
		Compile(ctx, schemaDoc)
	if err != nil {
		return nil, fmt.Errorf("compile invoice xsd: %w", err)
	}
	return schema, nil
})

// XSD validates xmlBytes against the GİB UBL-TR Invoice XSD.
func XSD(ctx context.Context, xmlBytes []byte) (Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	schema, err := compileInvoice()
	if err != nil {
		return Result{}, err
	}

	doc, err := helium.NewParser().Parse(ctx, xmlBytes)
	if err != nil {
		return Result{}, fmt.Errorf("parse xml: %w", err)
	}

	collector := helium.NewErrorCollector(ctx, helium.ErrorLevelNone)
	err = xsd.NewValidator(schema).ErrorHandler(collector).Validate(ctx, doc)
	var r Result
	for _, e := range collector.Errors() {
		r.Violations = append(r.Violations, xsdViolation(e))
	}
	if len(r.Violations) > 0 {
		return r, nil
	}
	if err != nil {
		return Result{}, fmt.Errorf("validate xml: %w", err)
	}
	return r, nil
}

func xsdViolation(err error) Violation {
	v := Violation{
		Layer:    "xsd",
		Severity: "error",
		Message:  strings.TrimSpace(err.Error()),
	}
	var ve *xsd.ValidationError
	if errors.As(err, &ve) {
		v.Message = strings.TrimSpace(ve.Message)
		v.Path = ve.Element
		if ve.Line > 0 {
			v.Path = fmt.Sprintf("%s:%d", ve.Element, ve.Line)
		}
	}
	return v
}
