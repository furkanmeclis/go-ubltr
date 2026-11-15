package edm

import (
	"bytes"
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"strings"

	"github.com/furkanmeclis/go-ubltr/provider"
	"github.com/furkanmeclis/go-ubltr/provider/soap"
)

//go:embed errors.csv
var errorsCSV string

// Codes is the EDM status table (errors.csv). EDM publishes no ERROR_CODE
// list; fault codes not in the table keep their raw value.
var Codes = provider.MustParseCodeTable(Name, errorsCSV)

func (p *Provider) header(session string) requestHeader {
	return requestHeader{
		SessionID:       session,
		ClientTxnID:     txnID(),
		ActionDate:      p.now().Format(actionDateTimeForm),
		Reason:          p.cfg.Reason,
		ApplicationName: p.cfg.ApplicationName,
		Hostname:        p.cfg.Hostname,
		ChannelName:     p.cfg.ChannelName,
		Compressed:      compressedNo,
	}
}

func txnID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func (p *Provider) sessionID(ctx context.Context) (string, uint64, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.session != "" {
		return p.session, p.gen, nil
	}
	id, err := p.login(ctx)
	if err != nil {
		return "", 0, err
	}
	p.session = id
	p.gen++
	return p.session, p.gen, nil
}

func (p *Provider) invalidate(gen uint64) {
	p.mu.Lock()
	if p.gen == gen {
		p.session = ""
	}
	p.mu.Unlock()
}

func (p *Provider) login(ctx context.Context) (string, error) {
	req := loginRequest{NS: nsTempuri, Header: p.header(""), UserName: p.cfg.Username, Password: p.cfg.Password}
	var resp loginResponse
	if err := p.client.Call(ctx, actionLogin, req, &resp); err != nil {
		e := p.wrap("login", err)
		// A fault on Login is treated as a credential problem.
		if isFault(e) && (e.Kind == provider.KindUnknown || e.Kind == provider.KindTransport) {
			e.Kind = provider.KindAuth
		}
		return "", e
	}
	id := strings.TrimSpace(resp.SessionID)
	if id == "" {
		return "", &provider.Error{Kind: provider.KindAuth, Provider: Name, Op: "login", Message: "empty SESSION_ID"}
	}
	return id, nil
}

func isFault(e *provider.Error) bool {
	var f *soap.Fault
	return errors.As(e, &f)
}

// Logout closes the current session, if any.
func (p *Provider) Logout(ctx context.Context) error {
	p.mu.Lock()
	id := p.session
	p.session = ""
	p.mu.Unlock()
	if id == "" {
		return nil
	}
	if err := p.client.Call(ctx, actionLogout, logoutRequest{NS: nsTempuri, Header: p.header(id)}, nil); err != nil {
		return p.wrap("logout", err)
	}
	return nil
}

// invoke runs one authenticated call; a session fault triggers one new
// login and a retry.
func invoke[R any](ctx context.Context, p *Provider, op, action string, build func(requestHeader) any) (*R, error) {
	for attempt := 0; ; attempt++ {
		id, gen, err := p.sessionID(ctx)
		if err != nil {
			return nil, err
		}
		out := new(R)
		err = p.client.Call(ctx, action, build(p.header(id)), out)
		if err == nil {
			return out, nil
		}
		e := p.wrap(op, err)
		if attempt == 0 && isSessionError(e) {
			p.invalidate(gen)
			continue
		}
		return nil, e
	}
}

// isSessionError recognizes an expired or unknown session. EDM does not
// document its session error code, so the fault text is checked.
func isSessionError(e *provider.Error) bool {
	if e.Op == "login" || !isFault(e) {
		return false
	}
	m := strings.ToLower(e.Message)
	return strings.Contains(m, "session") || strings.Contains(m, "oturum")
}

// wrap names op and reads RequestFault details from SOAP faults.
func (p *Provider) wrap(op string, err error) *provider.Error {
	var perr *provider.Error
	if !errors.As(err, &perr) {
		return provider.TransportError(Name, op, err)
	}
	out := *perr
	out.Op = op
	var f *soap.Fault
	if errors.As(err, &f) {
		if et := faultDetail(f.Detail); et != nil {
			msg := et.message()
			if msg == "" {
				msg = out.Message
			}
			e := p.codes.NewError(Name, op, strings.TrimSpace(et.Code), msg)
			if e.Kind == provider.KindUnknown {
				e.Kind = out.Kind
			}
			e.HTTPStatus, e.Raw, e.Err = out.HTTPStatus, out.Raw, f
			return e
		}
	}
	return &out
}

// faultDetail reads REQUEST_ERRORType fields from a RequestFault detail.
func faultDetail(detail string) *errorType {
	if strings.TrimSpace(detail) == "" {
		return nil
	}
	dec := xml.NewDecoder(bytes.NewReader([]byte("<d>" + detail + "</d>")))
	dec.Strict = false
	var et errorType
	found := false
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		var dst *string
		switch se.Name.Local {
		case "ERROR_CODE":
			dst, found = &et.Code, true
		case "ERROR_SHORT_DES":
			dst = &et.Short
		case "ERROR_LONG_DES":
			dst = &et.Long
		default:
			continue
		}
		var v string
		if err := dec.DecodeElement(&v, &se); err != nil {
			break
		}
		if *dst == "" {
			*dst = strings.TrimSpace(v)
		}
	}
	if !found {
		return nil
	}
	return &et
}
