package qnb

import (
	_ "embed"
	"errors"
	"strings"

	"github.com/furkanmeclis/go-ubltr/provider"
	"github.com/furkanmeclis/go-ubltr/provider/soap"
)

//go:embed errors.csv
var codesCSV string

// Codes is the QNB code table (status fields, e-Arşiv result codes and
// known fault texts).
var Codes = provider.MustParseCodeTable(Name, codesCSV)

// faultRules are the fault: rows of Codes, longest key first.
var faultRules = func() []provider.CodeInfo {
	var out []provider.CodeInfo
	for _, c := range Codes.Codes() {
		if strings.HasPrefix(c.Code, "fault:") {
			out = append(out, c)
		}
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && len(out[j].Code) > len(out[j-1].Code); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}()

// classify reclassifies SOAP faults using the fault: rows. The service
// raises a generic Exception fault whose text carries the reason.
func classify(err error, op string) error {
	var pe *provider.Error
	if !errors.As(err, &pe) {
		return err
	}
	pe.Op = op
	var f *soap.Fault
	if !errors.As(pe.Err, &f) {
		return pe
	}
	text := strings.ToLower(f.Reason + " " + f.Detail)
	for _, r := range faultRules {
		if strings.Contains(text, strings.ToLower(strings.TrimPrefix(r.Code, "fault:"))) {
			pe.Kind = r.Kind
			pe.Retryable = r.Retryable
			if r.Code == "fault:TPS_POSTA_KUTUSU_YETKI" {
				pe.GIBCode = 1172
			}
			return pe
		}
	}
	return pe
}

// resultError builds an error for an e-Arşiv resultCode other than
// AE00000.
func resultError(op, code, text string, raw []byte) error {
	if code == "AE00000" {
		return nil
	}
	e := Codes.NewError(Name, op, code, text)
	e.Raw = raw
	return e
}

func invalid(op, msg string) error {
	return &provider.Error{Kind: provider.KindInvalidRequest, Provider: Name, Op: op, Message: msg}
}
