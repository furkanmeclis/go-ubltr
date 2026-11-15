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

const memPrefix = "mem:/"

var schemaRel = map[Kind]string{
	KindInvoice:             "ubltr-1.2.1/xsdrt/maindoc/UBL-Invoice-2.1.xsd",
	KindCreditNote:          "ubltr-1.2.1/xsdrt/maindoc/UBL-CreditNote-2.1.xsd",
	KindApplicationResponse: "ubltr-1.2.1/xsdrt/maindoc/UBL-ApplicationResponse-2.1.xsd",
	KindDespatchAdvice:      "ubltr-1.2.1/xsdrt/maindoc/UBL-DespatchAdvice-2.1.xsd",
	KindReceiptAdvice:       "ubltr-1.2.1/xsdrt/maindoc/UBL-ReceiptAdvice-2.1.xsd",
	KindEArchiveReport:      "earchive/EArsiv.xsd",
	KindEnvelope:            envelopeRoot,
}

// The vendored GİB envelope XSDs import the UBL maindoc schemas by bare file
// name (UBL-Invoice-2.1.xsd) and an HR-XML schema that is not part of the
// package. uriFS serves the envelope files as if they sat next to the UBL
// maindoc schemas, so those relative imports resolve without editing the
// vendored files, and answers the HR-XML import with an empty schema for its
// namespace (user-account documents are not supported).
const (
	maindocDir   = "ubltr-1.2.1/xsdrt/maindoc/"
	envelopeDir  = "ubltr-1.2.1/envelope/"
	envelopeRoot = maindocDir + "PackageProxy_1_2.xsd"
	hrxmlStub    = "ubltr-1.2.1/xsdrt/HRXML/UserAccount.xsd"
)

const hrxmlStubXSD = `<?xml version="1.0" encoding="UTF-8"?>
<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema" targetNamespace="http://www.hr-xml.org/3"/>
`

// uriFS serves embed paths when helium asks for mem:/… schema locations.
// Open is unsupported so helium uses ReadFile for names that are not fs.ValidPath.
type uriFS struct {
	inner fs.FS
}

func (uriFS) Open(string) (fs.File, error) {
	return nil, fs.ErrNotExist
}

func (f uriFS) ReadFile(name string) ([]byte, error) {
	return readSchemaFile(f.inner, strings.TrimPrefix(name, memPrefix))
}

func readSchemaFile(fsys fs.FS, rel string) ([]byte, error) {
	data, err := fs.ReadFile(fsys, rel)
	if err == nil || !errors.Is(err, fs.ErrNotExist) {
		return data, err
	}
	if rel == hrxmlStub {
		return []byte(hrxmlStubXSD), nil
	}
	if base, ok := strings.CutPrefix(rel, maindocDir); ok && !strings.Contains(base, "/") {
		if data, eerr := fs.ReadFile(fsys, envelopeDir+base); eerr == nil {
			return data, nil
		}
	}
	return nil, err
}

var (
	schemaMu sync.Mutex
	schemasX = map[string]*xsd.Schema{}
)

func compileSchema(rel string) (*xsd.Schema, error) {
	schemaMu.Lock()
	defer schemaMu.Unlock()
	if s, ok := schemasX[rel]; ok {
		return s, nil
	}

	ctx := context.Background()
	schemaData, err := readSchemaFile(schemas.FS, rel)
	if err != nil {
		return nil, fmt.Errorf("read xsd %s: %w", rel, err)
	}

	uri := memPrefix + rel
	p := helium.NewParser().BaseURI(uri)
	schemaDoc, err := p.Parse(ctx, schemaData)
	if err != nil {
		return nil, fmt.Errorf("parse xsd %s: %w", rel, err)
	}

	schema, err := xsd.NewCompiler().
		FS(uriFS{inner: schemas.FS}).
		BaseDir(uri).
		Label(uri).
		Parser(p).
		Compile(ctx, schemaDoc)
	if err != nil {
		return nil, fmt.Errorf("compile xsd %s: %w", rel, err)
	}
	schemasX[rel] = schema
	return schema, nil
}

// XSD validates xmlBytes against the XSD for the detected document kind.
func XSD(ctx context.Context, xmlBytes []byte) (Result, error) {
	kind, err := Detect(xmlBytes)
	if err != nil {
		return Result{}, err
	}
	return XSDFor(ctx, xmlBytes, kind)
}

// XSDFor validates xmlBytes against the XSD for kind.
func XSDFor(ctx context.Context, xmlBytes []byte, kind Kind) (Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	rel, ok := schemaRel[kind]
	if !ok {
		return Result{}, fmt.Errorf("%w: %s", ErrUnknownDocument, kind)
	}

	schema, err := compileSchema(rel)
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
	loc := newNodeLocator(defaultPrefixes)
	for _, e := range collector.Errors() {
		r.Violations = append(r.Violations, xsdViolation(e, doc, loc))
	}
	if len(r.Violations) > 0 {
		return r, nil
	}
	if err != nil {
		return Result{}, fmt.Errorf("validate xml: %w", err)
	}
	return r, nil
}

// WellFormed reports whether data is namespace-well-formed XML (for example,
// every prefix is declared).
func WellFormed(ctx context.Context, data []byte) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, err := helium.NewParser().Parse(ctx, data); err != nil {
		return fmt.Errorf("parse xml: %w", err)
	}
	return nil
}

func xsdViolation(err error, doc *helium.Document, loc nodeLocator) Violation {
	v := Violation{
		Layer:    LayerXSD,
		Severity: SeverityError,
		Message:  strings.TrimSpace(err.Error()),
	}
	var lv helium.ErrorLeveler
	if errors.As(err, &lv) && lv.ErrorLevel() == helium.ErrorLevelWarning {
		v.Severity = SeverityWarning
	}
	var ve *xsd.ValidationError
	if errors.As(err, &ve) {
		v.Message = strings.TrimSpace(ve.Message)
		v.Path = ve.Element
		v.Element = ve.Element
		v.Attribute = ve.AttributeName
		v.Line = ve.Line
		if ve.Line > 0 {
			v.Path = fmt.Sprintf("%s:%d", ve.Element, ve.Line)
		}
		if n := findElementAtLine(doc, ve.Line, ve.Element); n != nil {
			v.Element = loc.qname(n)
			v.Location = loc.path(n)
			if ve.AttributeName != "" {
				v.Location += "/@" + ve.AttributeName
			}
		}
	}
	return v
}
