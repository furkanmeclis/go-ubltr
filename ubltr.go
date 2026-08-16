// Package ubltr generates and validates GİB UBL-TR electronic invoice XML.
//
// UBL-TR schema definitions are the source of truth. Generated Go types are
// derived from the schemas and must not be edited by hand.
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

// XSDValidationError is returned by [ValidateXSD] when the document is well-formed
// XML but does not satisfy the GİB UBL-TR Invoice schema.
type XSDValidationError struct {
	Messages []string
}

func (e *XSDValidationError) Error() string {
	if e == nil || len(e.Messages) == 0 {
		return "xsd validation failed"
	}
	return "xsd validation failed: " + strings.Join(e.Messages, "; ")
}

// ValidateXSD checks xmlBytes against the GİB UBL-TR Invoice XSD.
func ValidateXSD(ctx context.Context, xmlBytes []byte) error {
	result, err := validation.XSD(ctx, xmlBytes)
	if err != nil {
		return fmt.Errorf("validate xsd: %w", err)
	}
	if result.Valid() {
		return nil
	}
	return &XSDValidationError{Messages: violationMessages(result)}
}

// SchematronValidationError is returned by [ValidateSchematron] when the document
// does not satisfy GİB UBL-TR business rules.
type SchematronValidationError struct {
	Messages []string
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
	return &SchematronValidationError{Messages: violationMessages(result)}
}

// Validate runs Invoice XSD validation then GİB Schematron (e-Fatura).
func Validate(ctx context.Context, xmlBytes []byte) error {
	if err := ValidateXSD(ctx, xmlBytes); err != nil {
		return err
	}
	return ValidateSchematron(ctx, xmlBytes)
}

func violationMessages(result validation.Result) []string {
	msgs := make([]string, 0, len(result.Violations))
	for _, v := range result.Violations {
		msgs = append(msgs, v.String())
	}
	return msgs
}
