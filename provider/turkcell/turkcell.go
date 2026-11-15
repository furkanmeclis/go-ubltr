// Package turkcell is a provider module for the Turkcell e-Şirket
// e-Fatura / e-Arşiv REST API (efaturaservice).
//
// The API is documented at https://developer.turkcellesirket.com/fatura
// (Redoc / OpenAPI) with authentication at
// https://developer.turkcellesirket.com/pages/authentication.html and rate
// limits at https://developer.turkcellesirket.com/pages/rate-limits.html.
// Requests carry the API key in the "x-api-key" header.
package turkcell

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
const Name = "turkcell"

// Hosts from the OpenAPI servers list. Operations are under /v1 or /v2 as
// the documentation specifies per endpoint.
const (
	LiveURL = "https://efaturaservice.turkcellesirket.com"
	TestURL = "https://efaturaservicetest.isim360.com"
)

// maxResponse limits a response body (64 MiB).
const maxResponse = 64 << 20

//go:embed errors.csv
var codesCSV string

// Codes is the Turkcell invoice status code table.
var Codes = provider.MustParseCodeTable(Name, codesCSV)

// Config configures a Client.
type Config struct {
	// APIKey is created under API Yönetimi in the portal
	// (https://portaltest.isim360.com for test). Sent as x-api-key.
	APIKey string
	// Test selects https://efaturaservicetest.isim360.com when Endpoint is
	// empty.
	Test bool
	// Endpoint overrides the base URL (scheme and host, no /v1 or /v2).
	Endpoint string
	// StandardXSLT is the isStandartXslt path value for PDF and HTML
	// downloads. false renders with the account's default XSLT.
	StandardXSLT bool
	// HTTP overrides http.DefaultClient.
	HTTP provider.Doer
}

// Client talks to the Turkcell e-Şirket e-Fatura API. It is safe for
// concurrent use.
type Client struct {
	key      string
	base     string
	standard bool
	http     provider.Doer
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
	c := &Client{key: cfg.APIKey, base: strings.TrimRight(base, "/"), standard: cfg.StandardXSLT, http: cfg.HTTP}
	if c.http == nil {
		c.http = http.DefaultClient
	}
	return c, nil
}

func init() {
	provider.Register(Name, func(cfg provider.Config) (provider.Provider, error) {
		std, _ := strconv.ParseBool(cfg.Options["standard_xslt"])
		return New(Config{APIKey: cfg.APIKey, Test: cfg.Test, Endpoint: cfg.Endpoint, StandardXSLT: std, HTTP: cfg.HTTP})
	})
}

// Name implements provider.Provider.
func (c *Client) Name() string { return Name }

// Capabilities implements provider.Provider.
func (c *Client) Capabilities() provider.Capabilities {
	return provider.Capabilities{
		Operations: map[provider.DocumentKind][]provider.Operation{
			provider.EInvoice: {
				provider.OpSend, provider.OpStatus, provider.OpListInbox, provider.OpFetch,
				provider.OpRespond, provider.OpMarkTaken,
			},
			provider.EArchive: {
				provider.OpSend, provider.OpStatus, provider.OpFetch, provider.OpCancelEArchive,
			},
		},
		Formats:        []provider.Format{provider.FormatUBL, provider.FormatPDF, provider.FormatHTML},
		Protocol:       "rest",
		AssignsNumbers: true,
		SignsDocuments: true,
		AcceptsRawUBL:  true,
		MaxBatch:       1,
		// GET /v1/inboxinvoice/list is limited to 6 requests per 15 minutes.
		MinPollInterval: 150 * time.Second,
		TestEndpoint:    true,
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
	req.Header.Set("x-api-key", c.key)
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

// apiError classifies by HTTP status (401 auth, 403 permission, 404 not
// found, 422 validation, 429 rate limit) and keeps the documented 422
// body {"Error": {"title", "detail"}} as the message.
func apiError(op string, status int, body []byte) *provider.Error {
	e := provider.HTTPError(Name, op, status, body)
	if status == http.StatusUnprocessableEntity {
		// The docs describe 422 as records failing validation that must
		// not be resent unchanged.
		e.Kind = provider.KindValidation
	}
	var eb struct {
		Error *struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		} `json:"Error"`
	}
	if json.Unmarshal(body, &eb) == nil && eb.Error != nil {
		switch {
		case eb.Error.Title != "" && eb.Error.Detail != "":
			e.Message = eb.Error.Title + ": " + eb.Error.Detail
		case eb.Error.Title != "":
			e.Message = eb.Error.Title
		case eb.Error.Detail != "":
			e.Message = eb.Error.Detail
		}
	}
	return e
}

// flexInt decodes an integer sent as a JSON number or string.
type flexInt struct {
	set bool
	n   int
	raw string
}

func (f *flexInt) UnmarshalJSON(b []byte) error {
	s := strings.Trim(strings.TrimSpace(string(b)), `"`)
	if s == "" || s == "null" {
		return nil
	}
	f.raw = s
	if n, err := strconv.Atoi(s); err == nil {
		f.set, f.n = true, n
	}
	return nil
}

func parseTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	for _, l := range []string{time.RFC3339Nano, "2006-01-02T15:04:05.9999999", "2006-01-02T15:04:05", "2006-01-02 15:04:05", "2006-01-02"} {
		if t, err := time.Parse(l, s); err == nil {
			return t
		}
	}
	return time.Time{}
}
