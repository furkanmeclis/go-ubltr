package izibiz

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"strings"

	"github.com/furkanmeclis/go-ubltr/provider"
	"github.com/furkanmeclis/go-ubltr/provider/soap"
)

// İzibiz keeps a session for 8 hours and asks for a new Login when it
// reports "Aktif session bulunamadı" (Developer Portal, Kimlik Doğrulama).
// The module logs in lazily, shares the session between goroutines and
// logs in again once when a call fails with a session error.

// loginSessionID is sent by the vendor sample before a session exists.
const loginSessionID = "-1"

func (p *Provider) header(session string, compressed bool) requestHeader {
	h := requestHeader{
		SessionID:       session,
		ClientTxnID:     txnID(),
		ApplicationName: p.cfg.ApplicationName,
		ChannelName:     p.cfg.ChannelName,
	}
	if compressed {
		h.Compressed = "Y"
	} else {
		h.Compressed = "N"
	}
	return h
}

func txnID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// sessionID returns the current session, logging in when there is none.
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

// invalidate drops the session obtained at generation gen, unless another
// goroutine already replaced it.
func (p *Provider) invalidate(gen uint64) {
	p.mu.Lock()
	if p.gen == gen {
		p.session = ""
	}
	p.mu.Unlock()
}

func (p *Provider) login(ctx context.Context) (string, error) {
	req := loginRequest{
		NS:       nsWSDL,
		Header:   p.header(loginSessionID, false),
		UserName: p.cfg.Username,
		Password: p.cfg.Password,
	}
	req.Header.Compressed = ""
	var resp loginResponse
	if err := p.auth.Call(ctx, "", req, &resp); err != nil {
		return "", p.wrap("login", err)
	}
	if resp.Error != nil {
		e := p.codeError("login", resp.Error)
		if e.Kind == provider.KindUnknown {
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

// Logout closes the current session, if any.
func (p *Provider) Logout(ctx context.Context) error {
	p.mu.Lock()
	id := p.session
	p.session = ""
	p.mu.Unlock()
	if id == "" {
		return nil
	}
	req := struct {
		XMLName xml.Name      `xml:"wsdl:LogoutRequest"`
		NS      string        `xml:"xmlns:wsdl,attr"`
		Header  requestHeader `xml:"REQUEST_HEADER"`
	}{NS: nsWSDL, Header: p.header(id, false)}
	var resp result
	if err := p.auth.Call(ctx, "", req, &resp); err != nil {
		return p.wrap("logout", err)
	}
	if resp.Error != nil {
		return p.codeError("logout", resp.Error)
	}
	return nil
}

// carrier is implemented by every response type through result.
type carrier interface{ errorType() *errorType }

// invoke runs one authenticated call. build receives a fresh header for
// every attempt; a session error triggers one new login and a retry.
func invoke[R any, PR interface {
	*R
	carrier
}](ctx context.Context, p *Provider, c *soap.Client, op string, build func(requestHeader) any, compressed bool) (*R, error) {
	for attempt := 0; ; attempt++ {
		id, gen, err := p.sessionID(ctx)
		if err != nil {
			return nil, err
		}
		out := new(R)
		err = c.Call(ctx, "", build(p.header(id, compressed)), out)
		var perr *provider.Error
		if err != nil {
			perr = p.wrap(op, err)
		} else if et := PR(out).errorType(); et != nil {
			perr = p.codeError(op, et)
		}
		if perr == nil {
			return out, nil
		}
		if attempt == 0 && isSessionError(perr) {
			p.invalidate(gen)
			continue
		}
		return nil, perr
	}
}

// sessionCodes are ERROR_CODE values that mean the session is gone:
// 10004 (portal table) and 2005 (vendor sample re-login).
var sessionCodes = map[string]bool{"10004": true, "2005": true}

func isSessionError(e *provider.Error) bool {
	if e.Op == "login" {
		return false
	}
	if sessionCodes[strings.TrimSpace(e.Code)] {
		return true
	}
	m := strings.ToLower(e.Message)
	return strings.Contains(m, "session") || strings.Contains(m, "oturum")
}

// codeError turns ERROR_TYPE into a *provider.Error.
func (p *Provider) codeError(op string, et *errorType) *provider.Error {
	return p.codes.NewError(Name, op, strings.TrimSpace(et.Code), et.message())
}

// wrap names op on a transport or fault error and reads RequestFault
// details (ERROR_CODE / ERROR_SHORT_DES) when present.
func (p *Provider) wrap(op string, err error) *provider.Error {
	var perr *provider.Error
	if !errors.As(err, &perr) {
		return provider.TransportError(Name, op, err)
	}
	out := *perr
	out.Op = op
	var f *soap.Fault
	if errors.As(err, &f) {
		if et := faultDetail(f.Detail); et != nil && strings.TrimSpace(et.Code) != "" {
			e := p.codeError(op, et)
			if e.Kind == provider.KindUnknown {
				e.Kind = out.Kind
			}
			e.HTTPStatus, e.Raw, e.Err = out.HTTPStatus, out.Raw, f
			return e
		}
	}
	return &out
}
