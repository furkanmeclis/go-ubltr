// Package nilvera is a provider module for the Nilvera e-Fatura and
// e-Arşiv REST API.
//
// The API is documented at https://developer.nilvera.com (OpenAPI 3
// fragments per endpoint; Markdown via the ".md" suffix and llms.txt).
// Requests carry the API key as "Authorization: Bearer <key>".
package nilvera

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/furkanmeclis/go-ubltr/provider"
)

// Name is the registry name of this module.
const Name = "nilvera"

// Base URLs from https://developer.nilvera.com/genel-bakis. Service
// prefixes (/einvoice, /earchive, /general) are appended.
const (
	LiveURL = "https://api.nilvera.com"
	TestURL = "https://apitest.nilvera.com"
)

// maxResponse limits a response body (64 MiB).
const maxResponse = 64 << 20

//go:embed errors.csv
var errorsCSV string

// Codes is the Nilvera error code table.
var Codes = provider.MustParseCodeTable(Name, errorsCSV)

// Config configures a Client.
type Config struct {
	// APIKey is the key created in the Nilvera portal (API Tanımları >
	// Yeni Anahtar). Test and live keys are separate.
	APIKey string
	// Test selects https://apitest.nilvera.com when Endpoint is empty.
	Test bool
	// Endpoint overrides the base URL (scheme and host, no service prefix).
	Endpoint string
	// HTTP overrides http.DefaultClient.
	HTTP provider.Doer
}

// Client talks to the Nilvera REST API. It is safe for concurrent use.
type Client struct {
	key  string
	base string
	http provider.Doer
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
	c := &Client{key: cfg.APIKey, base: strings.TrimRight(base, "/"), http: cfg.HTTP}
	if c.http == nil {
		c.http = http.DefaultClient
	}
	return c, nil
}

func init() {
	provider.Register(Name, func(cfg provider.Config) (provider.Provider, error) {
		return New(Config{APIKey: cfg.APIKey, Test: cfg.Test, Endpoint: cfg.Endpoint, HTTP: cfg.HTTP})
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

type response struct {
	status int
	header http.Header
	body   []byte
}

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

// errorBody is the documented error shape (https://developer.nilvera.com/hata-kodlari).
type errorBody struct {
	Message string `json:"Message"`
	Errors  []struct {
		Code        json.Number `json:"Code"`
		Description string      `json:"Description"`
		Detail      string      `json:"Detail"`
	} `json:"Errors"`
}

// apiError builds a *provider.Error from an HTTP status and the error
// body. The HTTP status sets the kind; a code from the code table refines
// it. Nilvera codes are not GİB codes, so GIBCode stays zero.
func apiError(op string, status int, body []byte) *provider.Error {
	e := provider.HTTPError(Name, op, status, body)
	var eb errorBody
	if json.Unmarshal(body, &eb) != nil {
		return e
	}
	if eb.Message != "" {
		e.Message = eb.Message
	}
	if len(eb.Errors) == 0 {
		return e
	}
	first := eb.Errors[0]
	code := first.Code.String()
	e.Code = code
	e.Message = joinMsg(first.Description, first.Detail)
	if info, ok := Codes.Lookup(code); ok && info.Kind != provider.KindUnknown && info.Kind != provider.KindOK {
		e.Kind = info.Kind
		e.Retryable = e.Retryable || info.Retryable
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
