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
//
// Schematron (GİB business rules) is not run. Native compilation of the official
// UBL-TR Schematron files is not supported yet; see README.
func ValidateXSD(ctx context.Context, xmlBytes []byte) error {
	result, err := validation.XSD(ctx, xmlBytes)
	if err != nil {
		return err
	}
	if result.Valid() {
		return nil
	}
	msgs := make([]string, 0, len(result.Violations))
	for _, v := range result.Violations {
		msgs = append(msgs, v.String())
	}
	return &XSDValidationError{Messages: msgs}
}
