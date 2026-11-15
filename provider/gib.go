package provider

import (
	_ "embed"
	"strconv"
)

//go:embed gib_codes.csv
var gibCodesCSV string

// KindOK marks code-table rows that describe a successful state.
const KindOK ErrorKind = "ok"

// GIBCodes is the GİB envelope status code table (1000–1300). Many
// integrators pass these codes through unchanged.
var GIBCodes = MustParseCodeTable("gib", gibCodesCSV)

// GIBStatus maps a GİB envelope status code to a Status.
func GIBStatus(code int) Status {
	if c, ok := GIBCodes.Lookup(strconv.Itoa(code)); ok {
		return c.Status
	}
	return StatusUnknown
}

// GIBError returns an *Error for a failing GİB code, or nil for success
// and in-progress codes.
func GIBError(providerName, op string, code int, message string) *Error {
	c, ok := GIBCodes.Lookup(strconv.Itoa(code))
	if !ok {
		if code == 0 {
			return nil
		}
		return &Error{Kind: KindUnknown, Provider: providerName, Op: op, GIBCode: code, Code: strconv.Itoa(code), Message: message}
	}
	if c.Kind == KindOK {
		return nil
	}
	if message == "" {
		message = c.Message
	}
	return &Error{Kind: c.Kind, Provider: providerName, Op: op, GIBCode: code, Code: strconv.Itoa(code), Message: message, Retryable: c.Retryable}
}
