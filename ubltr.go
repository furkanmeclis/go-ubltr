// Package ubltr generates and validates GİB UBL-TR electronic document XML.
//
// UBL-TR schema definitions are the source of truth. Generated Go types are
// derived from the schemas and must not be edited by hand. The fluent builder
// API writes those generated types; it is not a second document model.
package ubltr

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"strings"

	"github.com/furkanmeclis/go-ubltr/internal/validation"
	"github.com/furkanmeclis/go-ubltr/schema/ubltr121/invoice"
)

const (
	// TypeEFatura is the GİB Schematron type parameter for e-Fatura.
	TypeEFatura = "efatura"
	// TypeEArchive is the GİB Schematron type parameter for e-Arşiv.
	TypeEArchive = "earchive"
	// TypeGoruntuleme is the GİB Schematron type parameter for viewing. Invoice
	// ProfileID may then be any e-Fatura profile or EARSIVFATURA.
	TypeGoruntuleme = "goruntuleme"
)

// Validation layers reported in [Violation.Layer].
const (
	LayerXSD        = validation.LayerXSD
	LayerSchematron = validation.LayerSchematron
)

// Severities reported in [Violation.Severity]. Schematron asserts are errors
// and reports are warnings unless a role or flag attribute says otherwise.
// The vendored GİB Schematron only uses asserts, so every GİB violation is an
// error today.
const (
	SeverityError   = validation.SeverityError
	SeverityWarning = validation.SeverityWarning
	SeverityInfo    = validation.SeverityInfo
)

// Marshal encodes v as UBL XML with an XML declaration.
func Marshal(v any) ([]byte, error) {
	body, err := xml.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal ubl xml: %w", err)
	}
	out := make([]byte, 0, len(xml.Header)+len(body))
	out = append(out, xml.Header...)
	out = append(out, body...)
	out = append(out, '\n')
	return out, nil
}

// Unmarshal decodes UBL XML into v.
func Unmarshal(data []byte, v any) error {
	if err := xml.Unmarshal(data, v); err != nil {
		return fmt.Errorf("unmarshal ubl xml: %w", err)
	}
	return nil
}

// NewInvoice returns an empty UBL-TR Invoice document.
func NewInvoice() *invoice.Invoice {
	inv := &invoice.Invoice{}
	return inv
}

// Violation is one XSD or Schematron failure. Fields other than Layer,
// Severity, and Message are filled when the engine provides them.
type Violation struct {
	// Layer is [LayerXSD] or [LayerSchematron].
	Layer string `json:"layer"`
	// Path is a short hint kept for compatibility: the element name for
	// Schematron, element:line for XSD. Prefer Location and Line.
	Path string `json:"path,omitempty"`
	// RuleID is the Schematron rule id. GİB rules are abstract rules such as
	// ProfileIDCheck, pulled in with sch:extends.
	RuleID string `json:"ruleId,omitempty"`
	// Severity is [SeverityError], [SeverityWarning], or [SeverityInfo].
	Severity string `json:"severity"`
	// Message is the human-readable message (Turkish for GİB Schematron).
	Message string `json:"message"`

	// TestID is the id attribute of the failing sch:assert or sch:report.
	TestID string `json:"testId,omitempty"`
	// PatternID is the Schematron pattern id (invoice, despatchadvice, …).
	// Rules outside any pattern are reported under "general".
	PatternID string `json:"patternId,omitempty"`
	// Context is the Schematron rule context, as written in the schema.
	Context string `json:"context,omitempty"`
	// Test is the XPath expression of the failing assert or report.
	Test string `json:"test,omitempty"`
	// Role and Flag are the raw Schematron attributes, if present.
	Role string `json:"role,omitempty"`
	Flag string `json:"flag,omitempty"`
	// Location is an XPath-like path to the failing node using the GİB
	// Schematron prefixes (inv, desp, apr, recp, sh, ef, cac, cbc, ext, …),
	// for example /inv:Invoice/cac:AccountingCustomerParty/cac:Party/cac:PartyIdentification.
	// A position predicate is added only when siblings share the name.
	Location string `json:"location,omitempty"`
	// Line is the 1-based source line of the failing node, or 0 if unknown.
	Line int `json:"line,omitempty"`
	// Element is the qualified name of the failing element.
	Element string `json:"element,omitempty"`
	// Attribute is the failing attribute name, if any.
	Attribute string `json:"attribute,omitempty"`
}

// String formats v as "layer: message (location)".
func (v Violation) String() string {
	where := v.Location
	if where == "" {
		where = v.Path
	}
	if where != "" {
		return v.Layer + ": " + v.Message + " (" + where + ")"
	}
	return v.Layer + ": " + v.Message
}

// IsError reports whether v has error severity.
func (v Violation) IsError() bool {
	return v.Severity == "" || v.Severity == SeverityError
}

// Violations returns the violations carried by err when it is (or wraps) an
// [*XSDValidationError] or [*SchematronValidationError]. It returns nil for
// any other error, including malformed XML.
func Violations(err error) []Violation {
	var out []Violation
	var xe *XSDValidationError
	if errors.As(err, &xe) && xe != nil {
		out = append(out, xe.Violations...)
	}
	var se *SchematronValidationError
	if errors.As(err, &se) && se != nil {
		out = append(out, se.Violations...)
	}
	return out
}

func filterSeverity(vs []Violation, errorsOnly bool) []Violation {
	var out []Violation
	for _, v := range vs {
		if v.IsError() == errorsOnly {
			out = append(out, v)
		}
	}
	return out
}

// XSDValidationError is returned by [ValidateXSD] when the document is well-formed
// XML but does not satisfy the GİB UBL-TR schema for its detected document kind.
type XSDValidationError struct {
	Violations []Violation
	Messages   []string
}

func (e *XSDValidationError) Error() string {
	if e == nil || len(e.Messages) == 0 {
		return "xsd validation failed"
	}
	return "xsd validation failed: " + strings.Join(e.Messages, "; ")
}

// Errors returns the violations with error severity.
func (e *XSDValidationError) Errors() []Violation {
	if e == nil {
		return nil
	}
	return filterSeverity(e.Violations, true)
}

// Warnings returns the violations with warning or info severity.
func (e *XSDValidationError) Warnings() []Violation {
	if e == nil {
		return nil
	}
	return filterSeverity(e.Violations, false)
}

// ValidateXSD checks xmlBytes against the GİB XSD for the detected document
// kind (UBL-TR documents or the e-Arşiv report).
func ValidateXSD(ctx context.Context, xmlBytes []byte) error {
	result, err := validation.XSD(ctx, xmlBytes)
	if err != nil {
		return fmt.Errorf("validate xsd: %w", err)
	}
	if result.Valid() {
		return nil
	}
	v := publicViolations(result)
	return &XSDValidationError{Violations: v, Messages: violationMessages(v)}
}

// SchematronValidationError is returned by [ValidateSchematron] when the document
// does not satisfy GİB UBL-TR business rules.
type SchematronValidationError struct {
	Violations []Violation
	Messages   []string
}

func (e *SchematronValidationError) Error() string {
	if e == nil || len(e.Messages) == 0 {
		return "schematron validation failed"
	}
	return "schematron validation failed: " + strings.Join(e.Messages, "; ")
}

// Errors returns the violations with error severity.
func (e *SchematronValidationError) Errors() []Violation {
	if e == nil {
		return nil
	}
	return filterSeverity(e.Violations, true)
}

// Warnings returns the violations with warning or info severity.
func (e *SchematronValidationError) Warnings() []Violation {
	if e == nil {
		return nil
	}
	return filterSeverity(e.Violations, false)
}

// ValidateSchematron checks xmlBytes against GİB UBL-TR Schematron rules
// using type=efatura. Use [ValidateSchematronType] for e-Arşiv.
//
// The rules run are chosen from [Detect]: Invoice, ApplicationResponse,
// DespatchAdvice (e-İrsaliye), and ReceiptAdvice run their own GİB pattern
// plus the general code-list rules; an envelope runs every pattern, which
// also covers the documents inside its ef:Package. See [HasSchematron].
func ValidateSchematron(ctx context.Context, xmlBytes []byte) error {
	return ValidateSchematronType(ctx, xmlBytes, TypeEFatura)
}

// HasSchematron reports whether the vendored GİB Main Schematron has rules
// for kind. It is true for Invoice, ApplicationResponse, DespatchAdvice,
// ReceiptAdvice, and the envelope. CreditNote and the e-Arşiv report have no
// GİB Schematron pattern.
func HasSchematron(kind Kind) bool {
	return validation.HasSchematron(validation.Kind(kind))
}

// ValidateSchematronType checks xmlBytes against GİB UBL-TR Schematron.
// docType is [TypeEFatura], [TypeEArchive], or [TypeGoruntuleme]; it only
// affects ProfileID-related checks.
func ValidateSchematronType(ctx context.Context, xmlBytes []byte, docType string) error {
	result, err := validation.Schematron(ctx, xmlBytes, docType)
	if err != nil {
		return fmt.Errorf("validate schematron: %w", err)
	}
	if result.Valid() {
		return nil
	}
	v := publicViolations(result)
	return &SchematronValidationError{Violations: v, Messages: violationMessages(v)}
}

// Validate runs XSD validation for the detected document kind, then GİB
// Schematron (type=efatura) when [HasSchematron] is true for that kind:
// Invoice, ApplicationResponse, DespatchAdvice, ReceiptAdvice, and the GİB
// envelope. CreditNote and the e-Arşiv report are checked against XSD only.
func Validate(ctx context.Context, xmlBytes []byte) error {
	kind, err := Detect(xmlBytes)
	if err != nil {
		return err
	}
	if err := ValidateXSD(ctx, xmlBytes); err != nil {
		return err
	}
	if HasSchematron(kind) {
		return ValidateSchematron(ctx, xmlBytes)
	}
	return nil
}

func publicViolations(result validation.Result) []Violation {
	out := make([]Violation, 0, len(result.Violations))
	for _, v := range result.Violations {
		out = append(out, Violation{
			Layer:     v.Layer,
			Path:      v.Path,
			RuleID:    v.RuleID,
			Severity:  v.Severity,
			Message:   v.Message,
			TestID:    v.TestID,
			PatternID: v.PatternID,
			Context:   v.Context,
			Test:      v.Test,
			Role:      v.Role,
			Flag:      v.Flag,
			Location:  v.Location,
			Line:      v.Line,
			Element:   v.Element,
			Attribute: v.Attribute,
		})
	}
	return out
}

func violationMessages(vs []Violation) []string {
	msgs := make([]string, 0, len(vs))
	for _, v := range vs {
		if v.Path != "" {
			msgs = append(msgs, v.Layer+": "+v.Message+" ("+v.Path+")")
			continue
		}
		msgs = append(msgs, v.Layer+": "+v.Message)
	}
	return msgs
}
