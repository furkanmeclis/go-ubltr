package sovos

import (
	_ "embed"
	"encoding/xml"
	"errors"
	"strconv"
	"strings"

	"github.com/furkanmeclis/go-ubltr/provider"
	"github.com/furkanmeclis/go-ubltr/provider/soap"
)

//go:embed errors.csv
var codesCSV string

// Codes is the Sovos code table. Keys are prefixed: ef. (e-Fatura
// ProcessingFault), ea. (e-Arşiv error codes), eastatus. (e-Arşiv invoice
// statuses).
var Codes = provider.MustParseCodeTable(Name, codesCSV)

// codeError builds an *Error for a raw code looked up under prefix.
func codeError(prefix, op, code, msg string, raw []byte) *provider.Error {
	e := &provider.Error{Kind: provider.KindUnknown, Provider: Name, Op: op, Code: code, Message: msg, Raw: raw}
	if info, ok := Codes.Lookup(prefix + code); ok && info.Kind != provider.KindOK {
		e.Kind, e.Retryable = info.Kind, info.Retryable
		if e.Message == "" {
			e.Message = info.Message
		}
	}
	return e
}

// faultDetail is ProcessingFault (e-Fatura: Code, Message) or
// processingFaultType (e-Arşiv: Code, Text).
type faultDetail struct {
	Code    string `xml:"Code"`
	Message string `xml:"Message"`
	Text    string `xml:"Text"`
}

// classify turns a SOAP fault carrying a Sovos fault detail into a
// table-based error; HTTP 401 stays KindAuth.
func classify(err error, prefix, op string) error {
	var pe *provider.Error
	if !errors.As(err, &pe) {
		return err
	}
	pe.Op = op
	var f *soap.Fault
	if !errors.As(pe.Err, &f) || f.Detail == "" {
		return pe
	}
	var d faultDetail
	// The detail holds one fault element; wrap it so both shapes decode.
	if xml.Unmarshal([]byte("<d>"+f.Detail+"</d>"), &struct {
		XMLName xml.Name
		F       *faultDetail `xml:",any"`
	}{F: &d}) != nil || strings.TrimSpace(d.Code) == "" {
		return pe
	}
	msg := strings.TrimSpace(d.Message + d.Text)
	if msg == "" {
		msg = f.Reason
	}
	out := codeError(prefix, op, strings.TrimSpace(d.Code), msg, pe.Raw)
	out.HTTPStatus = pe.HTTPStatus
	out.Err = f
	return out
}

func invalid(op, msg string) error {
	return &provider.Error{Kind: provider.KindInvalidRequest, Provider: Name, Op: op, Message: msg}
}

func atoi(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}
