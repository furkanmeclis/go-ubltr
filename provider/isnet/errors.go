package isnet

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/furkanmeclis/go-ubltr/provider"
)

//go:embed errors.csv
var codesCSV string

// Codes is the İşNet code table (status enumerations, Result=Failed
// message rules and this module's own codes).
var Codes = provider.MustParseCodeTable(Name, codesCSV)

// CodeIPNotAllowed is the provider.Error Code of a call to an IP-filtered
// endpoint that got no answer in time.
const CodeIPNotAllowed = "ip-not-allowlisted"

// ErrIPNotAllowed is wrapped by ip-not-allowlisted errors, so
// errors.Is(err, isnet.ErrIPNotAllowed) works as well as IsIPNotAllowed.
var ErrIPNotAllowed = errors.New("isnet: no answer from an IP-filtered endpoint; the caller's IP is probably not allowlisted by İşNet")

// IsIPNotAllowed reports whether err is the ip-not-allowlisted error: a
// call to the live (IP-filtered) İşNet endpoint that timed out while
// connecting or waiting for the answer. İşNet drops traffic from IPs it
// has not allowlisted instead of answering with an error, so a timeout is
// the only signal. A one-off network problem looks the same; when every
// call fails this way, ask İşNet to allowlist the egress IP.
func IsIPNotAllowed(err error) bool {
	var pe *provider.Error
	if errors.As(err, &pe) && pe.Provider == Name && pe.Code == CodeIPNotAllowed {
		return true
	}
	return errors.Is(err, ErrIPNotAllowed)
}

// classifyTransport rewrites network-level failures. Timeouts on an
// IP-filtered endpoint become ip-not-allowlisted (KindPermission, not
// retryable); timeouts elsewhere become KindTimeout. Errors that reached
// HTTP are returned unchanged apart from Op.
func classifyTransport(ctx context.Context, err error, op string, ipFiltered bool) error {
	var pe *provider.Error
	if !errors.As(err, &pe) {
		return err
	}
	pe.Op = op
	if pe.HTTPStatus != 0 || pe.Raw != nil || pe.Err == nil {
		return pe
	}
	if errors.Is(pe.Err, context.Canceled) && ctx.Err() != nil {
		return pe
	}
	if !isTimeout(pe.Err) {
		return pe
	}
	if ipFiltered {
		info, _ := Codes.Lookup(CodeIPNotAllowed)
		return &provider.Error{
			Kind:     provider.KindPermission,
			Provider: Name,
			Op:       op,
			Code:     CodeIPNotAllowed,
			Message:  info.Message,
			Err:      fmt.Errorf("%w: %w", ErrIPNotAllowed, pe.Err),
		}
	}
	pe.Kind = provider.KindTimeout
	pe.Retryable = true
	return pe
}

func isTimeout(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
		return true
	}
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}

// messageRules are the message: rows of Codes, longest key first.
var messageRules = func() []provider.CodeInfo {
	var out []provider.CodeInfo
	for _, c := range Codes.Codes() {
		if strings.HasPrefix(c.Code, "message:") {
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

// failed builds the error for Result=Failed. The kind comes from the
// message: rows; the raw ErrorMessage is kept as Message.
func failed(op, message string, raw []byte) *provider.Error {
	e := Codes.NewError(Name, op, "Failed", strings.TrimSpace(message))
	e.Raw = raw
	text := strings.ToLower(message)
	for _, r := range messageRules {
		if strings.Contains(text, strings.TrimPrefix(r.Code, "message:")) {
			e.Kind = r.Kind
			e.Retryable = r.Retryable
			break
		}
	}
	return e
}

func invalid(op, msg string) error {
	return &provider.Error{Kind: provider.KindInvalidRequest, Provider: Name, Op: op, Message: msg}
}

func notFound(op, uuid string) error {
	return &provider.Error{Kind: provider.KindNotFound, Provider: Name, Op: op, Message: "no document with ETTN " + uuid}
}
