// Package ubltr generates and validates GİB UBL-TR electronic document XML.
//
// UBL-TR schema definitions are the source of truth. Generated Go types are
// derived from the schemas and must not be edited by hand. The fluent builder
// API writes those generated types; it is not a second document model.
package ubltr

import (
	"context"
	"encoding/xml"
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

// Violation is one XSD or Schematron failure. Path and RuleID are filled when
// the engine provides them.
type Violation struct {
	Layer    string
	Path     string
	RuleID   string
	Severity string
	Message  string
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

// ValidateSchematron checks xmlBytes against GİB UBL-TR Schematron rules
// using type=efatura. Use [ValidateSchematronType] for e-Arşiv.
func ValidateSchematron(ctx context.Context, xmlBytes []byte) error {
	return ValidateSchematronType(ctx, xmlBytes, TypeEFatura)
}

// ValidateSchematronType checks xmlBytes against GİB UBL-TR Schematron.
// docType is "efatura" or "earchive".
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

// Validate runs XSD validation for the detected document kind. Invoice
// documents then run GİB Schematron (e-Fatura). Other kinds, including the
// e-Arşiv report, have no Invoice Schematron in this module.
func Validate(ctx context.Context, xmlBytes []byte) error {
	kind, err := Detect(xmlBytes)
	if err != nil {
		return err
	}
	if err := ValidateXSD(ctx, xmlBytes); err != nil {
		return err
	}
	if kind == KindInvoice {
		return ValidateSchematron(ctx, xmlBytes)
	}
	return nil
}

func publicViolations(result validation.Result) []Violation {
	out := make([]Violation, 0, len(result.Violations))
	for _, v := range result.Violations {
		out = append(out, Violation{
			Layer:    v.Layer,
			Path:     v.Path,
			RuleID:   v.RuleID,
			Severity: v.Severity,
			Message:  v.Message,
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
