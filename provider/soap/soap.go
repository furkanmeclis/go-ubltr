// Package soap is a small SOAP 1.1 / 1.2 client for integrator web
// services. Request and response bodies are plain encoding/xml structs.
package soap

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/furkanmeclis/go-ubltr/provider"
)

// Version selects the SOAP envelope version.
type Version int

// Versions.
const (
	V11 Version = iota
	V12
)

const (
	ns11 = "http://schemas.xmlsoap.org/soap/envelope/"
	ns12 = "http://www.w3.org/2003/05/soap-envelope"
)

// MaxResponseSize limits a response body (128 MiB).
const MaxResponseSize = 128 << 20

// Client calls one SOAP endpoint.
type Client struct {
	// URL is the service endpoint.
	URL string
	// Version defaults to V11.
	Version Version
	// HTTP defaults to http.DefaultClient.
	HTTP provider.Doer
	// Header is an optional SOAP header element (e.g. *Security).
	Header any
	// Provider names the integrator in errors.
	Provider string
	// Prepare, when set, edits each request (extra HTTP headers, auth).
	Prepare func(*http.Request)
}

// envelope is written with a "soap" prefix instead of a default namespace
// so unqualified payload elements (elementFormDefault="unqualified", common
// in integrator WSDLs) do not inherit the SOAP envelope namespace.
type envelope struct {
	XMLName xml.Name
	NS      string      `xml:"xmlns:soap,attr"`
	Header  *headerElem `xml:",omitempty"`
	Body    bodyElem
}

type headerElem struct {
	XMLName xml.Name
	Content any `xml:",omitempty"`
}

type bodyElem struct {
	XMLName xml.Name
	Content any `xml:",omitempty"`
}

// Fault is a SOAP fault (1.1 or 1.2 fields filled as available).
type Fault struct {
	Code    string
	Reason  string
	Actor   string
	Detail  string
	Version Version
}

func (f *Fault) Error() string {
	s := "soap fault"
	if f.Code != "" {
		s += " " + f.Code
	}
	if f.Reason != "" {
		s += ": " + f.Reason
	}
	return s
}

type fault11 struct {
	Code   string `xml:"faultcode"`
	String string `xml:"faultstring"`
	Actor  string `xml:"faultactor"`
	Detail struct {
		Inner string `xml:",innerxml"`
	} `xml:"detail"`
}

type fault12 struct {
	Code struct {
		Value   string `xml:"Value"`
		Subcode struct {
			Value string `xml:"Value"`
		} `xml:"Subcode"`
	} `xml:"Code"`
	Reason struct {
		Text []string `xml:"Text"`
	} `xml:"Reason"`
	Detail struct {
		Inner string `xml:",innerxml"`
	} `xml:"Detail"`
}

// Call posts req inside a SOAP body and decodes the first body element into
// resp (which may be nil). A SOAP fault is returned as a *provider.Error
// with Kind KindTransport (or KindAuth for authentication faults) wrapping
// a *Fault; integrator modules usually reclassify it from Fault.Reason.
func (c *Client) Call(ctx context.Context, action string, req, resp any) error {
	raw, err := c.Do(ctx, action, req)
	if err != nil {
		return err
	}
	if resp == nil {
		return nil
	}
	if _, err := decodeBody(raw, resp); err != nil {
		return &provider.Error{Kind: provider.KindTransport, Provider: c.Provider, Op: action, Message: "decode response", Raw: raw, Err: err}
	}
	return nil
}

// Do is Call returning the raw response envelope (MTOM parts stripped).
func (c *Client) Do(ctx context.Context, action string, req any) ([]byte, error) {
	payload, err := c.Marshal(req)
	if err != nil {
		return nil, &provider.Error{Kind: provider.KindInvalidRequest, Provider: c.Provider, Op: action, Err: err}
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.URL, bytes.NewReader(payload))
	if err != nil {
		return nil, &provider.Error{Kind: provider.KindInvalidRequest, Provider: c.Provider, Op: action, Err: err}
	}
	if c.Version == V12 {
		ct := `application/soap+xml; charset=utf-8`
		if action != "" {
			ct += `; action="` + action + `"`
		}
		httpReq.Header.Set("Content-Type", ct)
	} else {
		httpReq.Header.Set("Content-Type", "text/xml; charset=utf-8")
		httpReq.Header.Set("SOAPAction", `"`+action+`"`)
	}
	if c.Prepare != nil {
		c.Prepare(httpReq)
	}
	doer := c.HTTP
	if doer == nil {
		doer = http.DefaultClient
	}
	res, err := doer.Do(httpReq)
	if err != nil {
		return nil, provider.TransportError(c.Provider, action, err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, MaxResponseSize+1))
	if err != nil {
		return nil, provider.TransportError(c.Provider, action, err)
	}
	if len(body) > MaxResponseSize {
		return nil, &provider.Error{Kind: provider.KindTransport, Provider: c.Provider, Op: action, Message: "response too large"}
	}
	body = stripMTOM(body)
	_, ferr := decodeBody(body, nil)
	if ferr != nil {
		var f *Fault
		if errors.As(ferr, &f) {
			kind := provider.KindTransport
			if res.StatusCode == http.StatusUnauthorized || looksLikeAuth(f.Reason) {
				kind = provider.KindAuth
			}
			return nil, &provider.Error{Kind: kind, Provider: c.Provider, Op: action, Code: f.Code, Message: f.Reason, HTTPStatus: res.StatusCode, Raw: body, Err: f}
		}
		if res.StatusCode >= 300 {
			return nil, provider.HTTPError(c.Provider, action, res.StatusCode, body)
		}
		return nil, &provider.Error{Kind: provider.KindTransport, Provider: c.Provider, Op: action, Message: "invalid soap response", HTTPStatus: res.StatusCode, Raw: body, Err: ferr}
	}
	if res.StatusCode >= 300 {
		return nil, provider.HTTPError(c.Provider, action, res.StatusCode, body)
	}
	return body, nil
}

// Marshal renders the SOAP envelope for req.
func (c *Client) Marshal(req any) ([]byte, error) {
	ns := ns11
	if c.Version == V12 {
		ns = ns12
	}
	env := envelope{
		XMLName: xml.Name{Local: "soap:Envelope"},
		NS:      ns,
		Body:    bodyElem{XMLName: xml.Name{Local: "soap:Body"}, Content: req},
	}
	if c.Header != nil {
		env.Header = &headerElem{XMLName: xml.Name{Local: "soap:Header"}, Content: c.Header}
	}
	var buf bytes.Buffer
	buf.WriteString(xml.Header)
	if err := xml.NewEncoder(&buf).Encode(env); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// decodeBody walks the envelope to the first Body child. A Fault is
// returned as *Fault; otherwise the element is decoded into resp when
// resp is not nil. Decoding from the envelope keeps namespace bindings.
func decodeBody(envelopeXML []byte, resp any) (bool, error) {
	dec := xml.NewDecoder(bytes.NewReader(envelopeXML))
	inBody := false
	for {
		tok, err := dec.Token()
		if err != nil {
			if errors.Is(err, io.EOF) {
				if !inBody {
					return false, errors.New("soap: no Body element")
				}
				return false, nil
			}
			return false, fmt.Errorf("soap: decode envelope: %w", err)
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			if ee, ok := tok.(xml.EndElement); ok && inBody && ee.Name.Local == "Body" {
				return false, nil
			}
			continue
		}
		if !inBody {
			if se.Name.Local == "Body" && (se.Name.Space == ns11 || se.Name.Space == ns12) {
				inBody = true
			}
			continue
		}
		if se.Name.Local == "Fault" && (se.Name.Space == ns11 || se.Name.Space == ns12) {
			return true, decodeFault(dec, se)
		}
		if resp == nil {
			return true, nil
		}
		return true, dec.DecodeElement(resp, &se)
	}
}

func decodeFault(dec *xml.Decoder, se xml.StartElement) *Fault {
	if se.Name.Space == ns12 {
		var f fault12
		if err := dec.DecodeElement(&f, &se); err != nil {
			return &Fault{Reason: "unparsable fault", Version: V12}
		}
		code := f.Code.Value
		if f.Code.Subcode.Value != "" {
			code += "/" + f.Code.Subcode.Value
		}
		return &Fault{Code: code, Reason: strings.Join(f.Reason.Text, " "), Detail: strings.TrimSpace(f.Detail.Inner), Version: V12}
	}
	var f fault11
	if err := dec.DecodeElement(&f, &se); err != nil {
		return &Fault{Reason: "unparsable fault"}
	}
	return &Fault{Code: f.Code, Reason: f.String, Actor: f.Actor, Detail: strings.TrimSpace(f.Detail.Inner)}
}

// DecodeBody decodes the first SOAP Body child of a raw envelope (as
// returned by Do) into resp.
func DecodeBody(envelopeXML []byte, resp any) error {
	_, err := decodeBody(envelopeXML, resp)
	return err
}

// stripMTOM extracts the XML part of a multipart (MTOM/XOP) response.
func stripMTOM(body []byte) []byte {
	trim := bytes.TrimSpace(body)
	if !bytes.HasPrefix(trim, []byte("--")) {
		return body
	}
	start := bytes.Index(trim, []byte("<"))
	if start < 0 {
		return body
	}
	end := bytes.LastIndex(trim, []byte("Envelope>"))
	if end < 0 {
		return trim[start:]
	}
	return trim[start : end+len("Envelope>")]
}

func looksLikeAuth(s string) bool {
	s = strings.ToLower(s)
	for _, k := range []string{"authent", "unauthor", "kullanıcı adı", "şifre", "parola", "password", "credential", "yetkisiz", "oturum"} {
		if strings.Contains(s, k) {
			return true
		}
	}
	return false
}

// UsernameToken is a WS-Security UsernameToken header (PasswordText).
type UsernameToken struct {
	XMLName  xml.Name `xml:"http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-secext-1.0.xsd Security"`
	Username string   `xml:"http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-secext-1.0.xsd UsernameToken>Username"`
	Password password `xml:"http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-secext-1.0.xsd UsernameToken>Password"`
}

type password struct {
	Type  string `xml:"Type,attr"`
	Value string `xml:",chardata"`
}

// NewUsernameToken returns a WS-Security header with a plain-text password.
func NewUsernameToken(user, pass string) *UsernameToken {
	return &UsernameToken{
		Username: user,
		Password: password{Type: "http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-username-token-profile-1.0#PasswordText", Value: pass},
	}
}
