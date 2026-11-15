package provider

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// CodeInfo describes one integrator or GİB code.
type CodeInfo struct {
	Code      string
	Kind      ErrorKind
	Status    Status // status implied by the code, StatusUnknown if none
	Retryable bool
	// Message is a short Turkish description; MessageEN is optional.
	Message   string
	MessageEN string
}

// CodeTable maps integrator codes to normalized information. Tables are
// usually parsed from a CSV file embedded in each integrator module so
// vendor code lists stay data, not logic.
type CodeTable struct {
	name  string
	codes map[string]CodeInfo
}

// NewCodeTable builds a table from entries.
func NewCodeTable(name string, entries ...CodeInfo) *CodeTable {
	t := &CodeTable{name: name, codes: make(map[string]CodeInfo, len(entries))}
	for _, e := range entries {
		t.codes[e.Code] = e
	}
	return t
}

// ParseCodeTable reads CSV with the header
//
//	code,kind,status,retryable,message,message_en
//
// Lines starting with # are comments. kind and status use the string
// values of ErrorKind and Status; empty status means StatusUnknown.
func ParseCodeTable(name string, r io.Reader) (*CodeTable, error) {
	cr := csv.NewReader(r)
	cr.Comment = '#'
	cr.FieldsPerRecord = -1
	cr.TrimLeadingSpace = true
	header, err := cr.Read()
	if err != nil {
		return nil, fmt.Errorf("provider: code table %s: %w", name, err)
	}
	want := []string{"code", "kind", "status", "retryable", "message"}
	for i, h := range want {
		if i >= len(header) || strings.TrimSpace(header[i]) != h {
			return nil, fmt.Errorf("provider: code table %s: header must start with %s", name, strings.Join(want, ","))
		}
	}
	t := NewCodeTable(name)
	line := 1
	for {
		rec, err := cr.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		line++
		if err != nil {
			return nil, fmt.Errorf("provider: code table %s line %d: %w", name, line, err)
		}
		if len(rec) < 5 {
			return nil, fmt.Errorf("provider: code table %s line %d: want at least 5 fields", name, line)
		}
		info := CodeInfo{
			Code:    strings.TrimSpace(rec[0]),
			Kind:    ErrorKind(strings.TrimSpace(rec[1])),
			Status:  Status(strings.TrimSpace(rec[2])),
			Message: strings.TrimSpace(rec[4]),
		}
		if info.Status == "" {
			info.Status = StatusUnknown
		}
		if !validKind(info.Kind) {
			return nil, fmt.Errorf("provider: code table %s line %d: unknown kind %q", name, line, info.Kind)
		}
		if !info.Status.Valid() {
			return nil, fmt.Errorf("provider: code table %s line %d: unknown status %q", name, line, info.Status)
		}
		if s := strings.TrimSpace(rec[3]); s != "" {
			b, err := strconv.ParseBool(s)
			if err != nil {
				return nil, fmt.Errorf("provider: code table %s line %d: retryable: %w", name, line, err)
			}
			info.Retryable = b
		}
		if len(rec) > 5 {
			info.MessageEN = strings.TrimSpace(rec[5])
		}
		if _, dup := t.codes[info.Code]; dup {
			return nil, fmt.Errorf("provider: code table %s line %d: duplicate code %q", name, line, info.Code)
		}
		t.codes[info.Code] = info
	}
	return t, nil
}

// MustParseCodeTable is ParseCodeTable that panics; for embedded tables.
func MustParseCodeTable(name, data string) *CodeTable {
	t, err := ParseCodeTable(name, strings.NewReader(data))
	if err != nil {
		panic(err)
	}
	return t
}

// Name returns the table name.
func (t *CodeTable) Name() string { return t.name }

// Lookup returns the entry for code.
func (t *CodeTable) Lookup(code string) (CodeInfo, bool) {
	if t == nil {
		return CodeInfo{}, false
	}
	c, ok := t.codes[strings.TrimSpace(code)]
	return c, ok
}

// Len returns the number of entries.
func (t *CodeTable) Len() int {
	if t == nil {
		return 0
	}
	return len(t.codes)
}

// Codes returns all entries (unordered).
func (t *CodeTable) Codes() []CodeInfo {
	out := make([]CodeInfo, 0, t.Len())
	for _, c := range t.codes {
		out = append(out, c)
	}
	return out
}

// NewError builds an *Error for an integrator code. Unknown codes become
// KindUnknown but keep Code and Message so tables can be extended later.
// When the code is a numeric GİB code missing from t, the GİB table is
// consulted.
func (t *CodeTable) NewError(providerName, op, code, message string) *Error {
	e := &Error{Kind: KindUnknown, Provider: providerName, Op: op, Code: code, Message: message}
	info, ok := t.Lookup(code)
	if !ok {
		if n, err := strconv.Atoi(strings.TrimSpace(code)); err == nil {
			if g, gok := GIBCodes.Lookup(strconv.Itoa(n)); gok {
				info, ok = g, true
				e.GIBCode = n
			}
		}
	}
	if ok && info.Kind != KindOK {
		e.Kind = info.Kind
		e.Retryable = info.Retryable
		if e.Message == "" {
			e.Message = info.Message
		}
	}
	return e
}

func validKind(k ErrorKind) bool {
	switch k {
	case "", KindOK, KindUnknown, KindAuth, KindPermission, KindValidation, KindSignature,
		KindDuplicate, KindNotFound, KindRecipient, KindQuotaExceeded, KindRateLimit,
		KindInvalidState, KindInvalidRequest, KindUnavailable, KindTimeout,
		KindTransport, KindUnsupported, KindRecipientResponse:
		return true
	}
	return false
}
