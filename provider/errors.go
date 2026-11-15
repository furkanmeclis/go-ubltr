package provider

import (
	"errors"
	"fmt"
	"strings"
)

// ErrorKind is a normalized error category.
type ErrorKind string

// Error kinds.
const (
	KindUnknown           ErrorKind = "unknown"
	KindAuth              ErrorKind = "auth"               // bad credentials / token
	KindPermission        ErrorKind = "permission"         // account may not do this
	KindValidation        ErrorKind = "validation"         // document rejected (XSD, Schematron, business)
	KindSignature         ErrorKind = "signature"          // imza / mühür problem
	KindDuplicate         ErrorKind = "duplicate"          // UUID or ID already used
	KindNotFound          ErrorKind = "not-found"          // document / user not found
	KindRecipient         ErrorKind = "recipient"          // receiver not e-Fatura user / bad alias
	KindQuotaExceeded     ErrorKind = "quota-exceeded"     // kontör / plan limit
	KindRateLimit         ErrorKind = "rate-limit"         // too many requests
	KindInvalidState      ErrorKind = "invalid-state"      // e.g. cancel window closed
	KindInvalidRequest    ErrorKind = "invalid-request"    // bad parameters
	KindUnavailable       ErrorKind = "unavailable"        // integrator or GİB down
	KindTimeout           ErrorKind = "timeout"            // deadline exceeded
	KindTransport         ErrorKind = "transport"          // network / HTTP / SOAP fault
	KindUnsupported       ErrorKind = "unsupported"        // operation not offered
	KindRecipientResponse ErrorKind = "recipient-response" // receiver system returned failure (GİB 1230…)
)

// Sentinel errors for errors.Is. Every *Error matches the sentinel of its
// Kind.
var (
	ErrUnknown           = &Error{Kind: KindUnknown}
	ErrAuth              = &Error{Kind: KindAuth}
	ErrPermission        = &Error{Kind: KindPermission}
	ErrValidation        = &Error{Kind: KindValidation}
	ErrSignature         = &Error{Kind: KindSignature}
	ErrDuplicate         = &Error{Kind: KindDuplicate}
	ErrNotFound          = &Error{Kind: KindNotFound}
	ErrRecipient         = &Error{Kind: KindRecipient}
	ErrQuotaExceeded     = &Error{Kind: KindQuotaExceeded}
	ErrRateLimit         = &Error{Kind: KindRateLimit}
	ErrInvalidState      = &Error{Kind: KindInvalidState}
	ErrInvalidRequest    = &Error{Kind: KindInvalidRequest}
	ErrUnavailable       = &Error{Kind: KindUnavailable}
	ErrTimeout           = &Error{Kind: KindTimeout}
	ErrTransport         = &Error{Kind: KindTransport}
	ErrUnsupported       = &Error{Kind: KindUnsupported}
	ErrRecipientResponse = &Error{Kind: KindRecipientResponse}
)

// Error is returned by integrator modules.
type Error struct {
	Kind     ErrorKind
	Provider string
	Op       string
	// Code and Message are the integrator's own values, unchanged.
	Code    string
	Message string
	// GIBCode is the GİB envelope/document status code when known.
	GIBCode int
	// HTTPStatus is the HTTP status when the call reached HTTP.
	HTTPStatus int
	// Retryable is true when the same request may succeed later.
	Retryable bool
	// Raw is the response body or fault, for diagnostics.
	Raw []byte
	// Err is the underlying cause.
	Err error
}

func (e *Error) Error() string {
	var b strings.Builder
	b.WriteString("provider")
	if e.Provider != "" {
		b.WriteString(" " + e.Provider)
	}
	if e.Op != "" {
		b.WriteString(" " + e.Op)
	}
	b.WriteString(": " + string(e.Kind))
	if e.Code != "" {
		b.WriteString(" [" + e.Code + "]")
	}
	if e.GIBCode != 0 {
		fmt.Fprintf(&b, " (GİB %d)", e.GIBCode)
	}
	if e.Message != "" {
		b.WriteString(": " + e.Message)
	}
	if e.Err != nil {
		b.WriteString(": " + e.Err.Error())
	}
	return b.String()
}

// Unwrap returns the cause.
func (e *Error) Unwrap() error { return e.Err }

// Is matches sentinels by Kind.
func (e *Error) Is(target error) bool {
	t, ok := target.(*Error)
	if !ok {
		return false
	}
	// Sentinels carry only Kind.
	if t.Provider == "" && t.Op == "" && t.Code == "" && t.Err == nil {
		return t.Kind == e.Kind
	}
	return e == t
}

// KindOf returns the Kind of err, or KindUnknown.
func KindOf(err error) ErrorKind {
	var e *Error
	if errors.As(err, &e) {
		return e.Kind
	}
	return KindUnknown
}

// IsRetryable reports whether err is an *Error marked retryable, or a kind
// that is always transient.
func IsRetryable(err error) bool {
	var e *Error
	if !errors.As(err, &e) {
		return false
	}
	if e.Retryable {
		return true
	}
	switch e.Kind {
	case KindRateLimit, KindUnavailable, KindTimeout:
		return true
	}
	return false
}
