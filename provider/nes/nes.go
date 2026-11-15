// Package nes is a provider module for the NES (nes.com.tr) e-Fatura and
// e-Arşiv REST API.
//
// The API is described by the OpenAPI files published at
// https://apitest.nes.com.tr/einvoice/index.html (e-Fatura),
// https://apitest.nes.com.tr/earchive/index.html (e-Arşiv) and the
// developer portal https://developertest.nes.com.tr/docs. Requests carry
// the API key as "Authorization: Bearer <key>".
package nes

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/go-ubltr/provider"
)

// Name is the registry name of this module.
const Name = "nes"

// Base URLs. Service prefixes (/einvoice, /earchive, /general) are appended.
const (
	LiveURL = "https://api.nes.com.tr"
	TestURL = "https://apitest.nes.com.tr"
)

// DefaultSourceApp is sent as the mandatory SourceApp upload field when
// Config.SourceApp is empty.
const DefaultSourceApp = "go-ubltr"

// MaxFileSize is the documented upload limit for the File field (1 MB).
// Larger XML is zipped before upload.
const MaxFileSize = 1 << 20

// MaxCancelBatch is the documented e-Arşiv cancel limit per request.
const MaxCancelBatch = 50

// maxResponse limits a response body (64 MiB).
const maxResponse = 64 << 20

//go:embed errors.csv
var errorsCSV string

// Codes is the NES error code table.
var Codes = provider.MustParseCodeTable(Name, errorsCSV)

// Config configures a Client.
type Config struct {
	// APIKey is the key created in the NES portal (Yönetim Paneli > API
	// Tanımları > Yeni API Anahtarı).
	APIKey string
	// Test selects https://apitest.nes.com.tr when Endpoint is empty.
	Test bool
	// Endpoint overrides the base URL (scheme and host, no service prefix).
	Endpoint string
	// SourceApp is the mandatory upload field naming the calling
	// application; DefaultSourceApp when empty.
	SourceApp string
	// SenderAlias is the default gönderici birim etiketi for e-Fatura when
	// Document.SenderAlias is empty.
	SenderAlias string
	// HTTP overrides http.DefaultClient.
	HTTP provider.Doer
}

// Client talks to the NES REST API. It is safe for concurrent use.
type Client struct {
	key         string
	base        string
	sourceApp   string
	senderAlias string
	http        provider.Doer
}

// New returns a Client.
func New(cfg Config) (*Client, error) {
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, &provider.Error{Kind: provider.KindAuth, Provider: Name, Op: "new", Message: "APIKey is required"}
	}
	base := cfg.Endpoint
	if base == "" {
		base = LiveURL
		if cfg.Test {
			base = TestURL
		}
	}
	c := &Client{
		key:         cfg.APIKey,
		base:        strings.TrimRight(base, "/"),
		sourceApp:   cfg.SourceApp,
		senderAlias: cfg.SenderAlias,
		http:        cfg.HTTP,
	}
	if c.sourceApp == "" {
		c.sourceApp = DefaultSourceApp
	}
	if c.http == nil {
		c.http = http.DefaultClient
	}
	return c, nil
}

func init() {
	provider.Register(Name, func(cfg provider.Config) (provider.Provider, error) {
		return New(Config{
			APIKey:      cfg.APIKey,
			Test:        cfg.Test,
			Endpoint:    cfg.Endpoint,
			SourceApp:   cfg.Options["source_app"],
			SenderAlias: cfg.Options["sender_alias"],
			HTTP:        cfg.HTTP,
		})
	})
}

// Name implements provider.Provider.
func (c *Client) Name() string { return Name }

// Capabilities implements provider.Provider.
func (c *Client) Capabilities() provider.Capabilities {
	return provider.Capabilities{
		Operations: map[provider.DocumentKind][]provider.Operation{
			provider.EInvoice: {
				provider.OpSend, provider.OpStatus, provider.OpListInbox, provider.OpListOutbox,
				provider.OpFetch, provider.OpRespond, provider.OpCheckUser, provider.OpMarkTaken,
				provider.OpCredits,
			},
			provider.EArchive: {
				provider.OpSend, provider.OpStatus, provider.OpListOutbox, provider.OpFetch,
				provider.OpCancelEArchive, provider.OpCredits,
			},
		},
		Formats:        []provider.Format{provider.FormatUBL, provider.FormatPDF, provider.FormatHTML},
		Protocol:       "rest",
		SignsDocuments: true,
		AcceptsRawUBL:  true,
		MaxBatch:       1,
		TestEndpoint:   true,
	}
}

// service returns the path prefix for a document kind.
func service(kind provider.DocumentKind) string {
	if kind == provider.EArchive {
		return "/earchive"
	}
	return "/einvoice"
}

type response struct {
	status int
	header http.Header
	body   []byte
}

// do sends a request and returns the body of a 2xx response, or a
// *provider.Error.
func (c *Client) do(ctx context.Context, op, method, path string, q url.Values, body io.Reader, contentType string) (*response, error) {
	u := c.base + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return nil, &provider.Error{Kind: provider.KindInvalidRequest, Provider: Name, Op: op, Err: err}
	}
	req.Header.Set("Authorization", "Bearer "+c.key)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, provider.TransportError(Name, op, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponse))
	if err != nil {
		return nil, provider.TransportError(Name, op, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, apiError(op, resp.StatusCode, data)
	}
	return &response{status: resp.StatusCode, header: resp.Header, body: data}, nil
}

func (c *Client) doJSON(ctx context.Context, op, method, path string, q url.Values, in, out any) (*response, error) {
	var body io.Reader
	ct := ""
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return nil, &provider.Error{Kind: provider.KindInvalidRequest, Provider: Name, Op: op, Err: err}
		}
		body, ct = bytes.NewReader(b), "application/json"
	}
	r, err := c.do(ctx, op, method, path, q, body, ct)
	if err != nil {
		return nil, err
	}
	if out != nil && len(bytes.TrimSpace(r.body)) > 0 {
		if err := json.Unmarshal(r.body, out); err != nil {
			return r, &provider.Error{Kind: provider.KindTransport, Provider: Name, Op: op, HTTPStatus: r.status, Raw: r.body, Message: "decode response", Err: err}
		}
	}
	return r, nil
}

// errorBody is the documented error shape: 400 carries invalidFields,
// 422 carries errors.
type errorBody struct {
	Message       string `json:"message"`
	InvalidFields []struct {
		Field       string `json:"field"`
		Description string `json:"description"`
		Detail      string `json:"detail"`
	} `json:"invalidFields"`
	Errors []struct {
		Code        string `json:"code"`
		Description string `json:"description"`
		Detail      string `json:"detail"`
	} `json:"errors"`
}

// apiError builds a *provider.Error from an HTTP status and the NES error
// body. The HTTP status sets the kind; a code from the code table refines
// it.
func apiError(op string, status int, body []byte) *provider.Error {
	e := provider.HTTPError(Name, op, status, body)
	var eb errorBody
	if json.Unmarshal(body, &eb) != nil {
		return e
	}
	if eb.Message != "" {
		e.Message = eb.Message
	}
	switch {
	case len(eb.Errors) > 0:
		first := eb.Errors[0]
		msg := first.Detail
		if msg == "" {
			msg = first.Description
		}
		ce := Codes.NewError(Name, op, first.Code, msg)
		e.Code = first.Code
		e.Message = joinMsg(first.Description, first.Detail)
		if ce.Kind != provider.KindUnknown {
			e.Kind = ce.Kind
			e.Retryable = ce.Retryable
		}
		if g, ok := provider.GIBCodes.Lookup(first.Code); ok && g.Kind != provider.KindOK {
			e.GIBCode, _ = strconv.Atoi(g.Code)
		}
	case len(eb.InvalidFields) > 0:
		parts := make([]string, 0, len(eb.InvalidFields))
		for _, f := range eb.InvalidFields {
			parts = append(parts, joinMsg(f.Field, f.Description))
		}
		e.Message = joinMsg(eb.Message, strings.Join(parts, "; "))
	}
	return e
}

func joinMsg(a, b string) string {
	switch {
	case a == "":
		return b
	case b == "":
		return a
	}
	return a + ": " + b
}

// parseTime accepts the date-time forms used by the API.
func parseTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	for _, l := range []string{time.RFC3339Nano, "2006-01-02T15:04:05.9999999", "2006-01-02T15:04:05", "2006-01-02"} {
		if t, err := time.Parse(l, s); err == nil {
			return t
		}
	}
	return time.Time{}
}
