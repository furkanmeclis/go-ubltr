package uyumsoft

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"time"

	"github.com/furkanmeclis/go-ubltr/provider"
	"github.com/furkanmeclis/go-ubltr/provider/soap"
)

// Scenario is the InvoiceInfo/Scenario value (WSDL InvoiceScenarioChoosen).
type Scenario string

// Scenarios.
const (
	// ScenarioAuto derives eInvoice / eArchive from the document kind.
	ScenarioAuto Scenario = ""
	// ScenarioAutomated lets the integrator decide from the GİB user list.
	ScenarioAutomated Scenario = "Automated"
	ScenarioEInvoice  Scenario = "eInvoice"
	ScenarioEArchive  Scenario = "eArchive"
)

// Config configures a Client.
type Config struct {
	// Username and Password are the web-service credentials. For
	// muhasebeizleme, Password is the API key and Username is informative.
	Username, Password string
	// Preset selects the vendor; the zero value means Uyumsoft.
	Preset Preset
	// Test selects the preset's test endpoint when Endpoint is empty.
	Test bool
	// Endpoint overrides the SOAP endpoint URL.
	Endpoint string
	// Scenario is the default InvoiceInfo/Scenario for Send.
	Scenario Scenario
	// NoTimestamp omits the wsu:Timestamp from the WS-Security header.
	NoTimestamp bool
	// HTTP overrides the HTTP client.
	HTTP provider.Doer
	// Now overrides time.Now (tests).
	Now func() time.Time
}

// Client talks to one Integration-compatible endpoint.
type Client struct {
	cfg    Config
	preset Preset
	soap   *soap.Client
	now    func() time.Time
}

var (
	_ provider.Provider          = (*Client)(nil)
	_ provider.Sender            = (*Client)(nil)
	_ provider.StatusChecker     = (*Client)(nil)
	_ provider.InboxLister       = (*Client)(nil)
	_ provider.OutboxLister      = (*Client)(nil)
	_ provider.Fetcher           = (*Client)(nil)
	_ provider.Responder         = (*Client)(nil)
	_ provider.EArchiveCanceller = (*Client)(nil)
	_ provider.UserChecker       = (*Client)(nil)
	_ provider.Acknowledger      = (*Client)(nil)
	_ provider.CreditChecker     = (*Client)(nil)
)

// New returns a Client.
func New(cfg Config) (*Client, error) {
	p := cfg.Preset
	if p.Name == "" {
		p = Uyumsoft
	}
	if cfg.Username == "" && p.Auth == AuthWSSecurity {
		return nil, errors.New("uyumsoft: Username is required")
	}
	if cfg.Password == "" {
		return nil, errors.New("uyumsoft: Password is required")
	}
	url := cfg.Endpoint
	if url == "" {
		url = p.URL(cfg.Test)
	}
	if url == "" {
		return nil, fmt.Errorf("uyumsoft: preset %s has no endpoint", p.Name)
	}
	c := &Client{cfg: cfg, preset: p, now: cfg.Now}
	if c.now == nil {
		c.now = time.Now
	}
	c.soap = &soap.Client{URL: url, Version: soap.V11, HTTP: cfg.HTTP, Provider: p.Name}
	return c, nil
}

// FromConfig builds a Client from the registry configuration. Options:
// "scenario" (Automated, eInvoice, eArchive) and "timestamp" ("false"
// disables the WS-Security Timestamp). APIKey is used as Password when
// Password is empty.
func FromConfig(p Preset, cfg provider.Config) (*Client, error) {
	c := Config{
		Username: cfg.Username,
		Password: cfg.Password,
		Preset:   p,
		Test:     cfg.Test,
		Endpoint: cfg.Endpoint,
		HTTP:     cfg.HTTP,
	}
	if c.Password == "" {
		c.Password = cfg.APIKey
	}
	if s, ok := cfg.Options["scenario"]; ok {
		switch Scenario(s) {
		case ScenarioAuto, ScenarioAutomated, ScenarioEInvoice, ScenarioEArchive:
			c.Scenario = Scenario(s)
		default:
			return nil, fmt.Errorf("uyumsoft: unknown scenario option %q", s)
		}
	}
	if s, ok := cfg.Options["timestamp"]; ok {
		b, err := strconv.ParseBool(s)
		if err != nil {
			return nil, fmt.Errorf("uyumsoft: timestamp option: %w", err)
		}
		c.NoTimestamp = !b
	}
	return New(c)
}

func init() {
	for _, p := range []Preset{Uyumsoft, Muhasebeizleme} {
		provider.Register(p.Name, func(cfg provider.Config) (provider.Provider, error) {
			return FromConfig(p, cfg)
		})
	}
}

// Name implements provider.Provider.
func (c *Client) Name() string { return c.preset.Name }

// Preset returns the vendor preset in use.
func (c *Client) Preset() Preset { return c.preset }

// Capabilities implements provider.Provider.
func (c *Client) Capabilities() provider.Capabilities {
	caps := c.preset.Capabilities
	caps.Operations = maps.Clone(caps.Operations)
	for k, v := range caps.Operations {
		caps.Operations[k] = slices.Clone(v)
	}
	caps.Formats = slices.Clone(caps.Formats)
	return caps
}

func (c *Client) supports(kind provider.DocumentKind, op provider.Operation) error {
	if kind == "" {
		kind = provider.EInvoice
	}
	if !c.preset.Capabilities.Has(kind, op) {
		return provider.Unsupported(c.preset.Name, op, kind)
	}
	return nil
}

// userInfo returns the body credential element for AuthUserInfo presets.
func (c *Client) userInfo() *userInfo {
	if c.preset.Auth != AuthUserInfo {
		return nil
	}
	return &userInfo{Username: c.cfg.Username, Password: c.cfg.Password}
}

// header returns the SOAP header for AuthWSSecurity presets.
func (c *Client) header() any {
	if c.preset.Auth != AuthWSSecurity {
		return nil
	}
	return newSecurity(c.cfg.Username, c.cfg.Password, c.now(), !c.cfg.NoTimestamp)
}

// resulter is implemented by every <OpResult> type.
type resulter interface {
	status() *result
}

// call posts req for op and decodes the <OpResult> element into a new T.
// A SOAP fault or IsSucceded="false" becomes a *provider.Error.
func call[T any, PT interface {
	*T
	resulter
}](ctx context.Context, c *Client, op string, req any) (PT, []byte, error) {
	sc := *c.soap
	sc.Header = c.header()
	raw, err := sc.Do(ctx, c.preset.action(op), req)
	if err != nil {
		return nil, nil, c.faultError(op, err)
	}
	var env struct {
		Result PT `xml:",any"`
	}
	if err := soap.DecodeBody(raw, &env); err != nil {
		return nil, raw, &provider.Error{Kind: provider.KindTransport, Provider: c.preset.Name, Op: op, Message: "decode response", Raw: raw, Err: err}
	}
	if env.Result == nil {
		return nil, raw, &provider.Error{Kind: provider.KindTransport, Provider: c.preset.Name, Op: op, Message: "empty response", Raw: raw}
	}
	if pr, ok := any(env.Result).(partialResult); ok && pr.partial() {
		return env.Result, raw, nil
	}
	if r := env.Result.status(); !r.IsSucceded {
		e := messageError(c.preset.Name, op, r.Message)
		e.Raw = raw
		return nil, raw, e
	}
	return env.Result, raw, nil
}

// partialResult is implemented by batch results whose items carry their
// own IsSucceded: an outer IsSucceded="false" with items present means
// some items failed, not the whole call.
type partialResult interface {
	partial() bool
}

// result is the common Response base type: IsSucceded / Message
// attributes (spelling as in the WSDL).
type result struct {
	IsSucceded bool   `xml:"IsSucceded,attr"`
	Message    string `xml:"Message,attr"`
}

func (r *result) status() *result { return r }

// flagResult is FlagResponse (Uyumsoft) / DisApiBayrakCevap
// (BasicIntegration): Value is a boolean attribute.
type flagResult struct {
	result
	Value *bool `xml:"Value,attr"`
}

// checkFlag turns Value="false" into an error classified from Message.
func (c *Client) checkFlag(op string, f *flagResult, raw []byte) error {
	if f.Value != nil && !*f.Value {
		msg := f.Message
		if msg == "" {
			msg = op + " returned false"
		}
		e := messageError(c.preset.Name, op, msg)
		e.Raw = raw
		return e
	}
	return nil
}

// userInfo is the BasicIntegration credential element (attribute form,
// valid for both UserInfo and UserInformation in that WSDL).
type userInfo struct {
	Username string `xml:"Username,attr,omitempty"`
	Password string `xml:"Password,attr"`
}

const (
	nsTempuri = "http://tempuri.org/"
	nsWSSE    = "http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-secext-1.0.xsd"
	nsWSU     = "http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-utility-1.0.xsd"
	pwText    = "http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-username-token-profile-1.0#PasswordText"
)

// security is a WS-Security header with an optional Timestamp and a
// plain-text UsernameToken (WssUsernameToken10 over HTTPS).
type security struct {
	XMLName   xml.Name   `xml:"http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-secext-1.0.xsd Security"`
	Timestamp *timestamp `xml:"http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-utility-1.0.xsd Timestamp,omitempty"`
	Token     struct {
		Username string `xml:"http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-secext-1.0.xsd Username"`
		Password struct {
			Type  string `xml:"Type,attr"`
			Value string `xml:",chardata"`
		} `xml:"http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-secext-1.0.xsd Password"`
	} `xml:"http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-secext-1.0.xsd UsernameToken"`
}

type timestamp struct {
	Created string `xml:"http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-utility-1.0.xsd Created"`
	Expires string `xml:"http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-utility-1.0.xsd Expires"`
}

// timestampTTL is the lifetime of the wsu:Timestamp (WCF default skew is
// five minutes).
const timestampTTL = 5 * time.Minute

func newSecurity(user, pass string, now time.Time, withTimestamp bool) *security {
	s := &security{}
	s.Token.Username = user
	s.Token.Password.Type = pwText
	s.Token.Password.Value = pass
	if withTimestamp {
		now = now.UTC()
		s.Timestamp = &timestamp{
			Created: now.Format("2006-01-02T15:04:05.000Z"),
			Expires: now.Add(timestampTTL).Format("2006-01-02T15:04:05.000Z"),
		}
	}
	return s
}

// trt is Türkiye time (UTC+3, no DST). Both vendors read and write
// xs:dateTime values without an offset as Türkiye time.
var trt = time.FixedZone("TRT", 3*60*60)

const localLayout = "2006-01-02T15:04:05"

func formatLocal(t time.Time) string { return t.In(trt).Format(localLayout) }

// parseTime reads an xs:dateTime; values without an offset are Türkiye time.
func parseTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	for _, l := range []string{time.RFC3339Nano, "2006-01-02T15:04:05.999999999"} {
		if t, err := time.ParseInLocation(l, s, trt); err == nil {
			return t
		}
	}
	for _, l := range []string{"02.01.2006 15:04:05", "2006-01-02 15:04:05", "2006-01-02", "02.01.2006"} {
		if t, err := time.ParseInLocation(l, s, trt); err == nil {
			return t
		}
	}
	return time.Time{}
}
