package trendyol

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/furkanmeclis/go-ubltr/provider"
)

//go:embed errors.csv
var codesCSV string

// Codes is the Trendyol code table: status.* (document statuses) and
// http.* (documented HTTP error statuses).
var Codes = provider.MustParseCodeTable(Name, codesCSV)

// maxBody limits a JSON response (32 MiB) and a download (128 MiB).
const (
	maxBody     = 32 << 20
	maxDownload = 128 << 20
)

// problem is the RFC 7807 error body.
type problem struct {
	Title  string `json:"title"`
	Detail string `json:"detail"`
	Status int    `json:"status"`
	Code   string `json:"code"`
}

// apiError classifies a non-2xx response with the http.* rows, keeping
// the problem title / detail / code.
func apiError(op string, status int, body []byte) *provider.Error {
	e := provider.HTTPError(Name, op, status, body)
	if info, ok := Codes.Lookup("http." + strconv.Itoa(status)); ok {
		e.Kind, e.Retryable = info.Kind, info.Retryable
	}
	var p problem
	if json.Unmarshal(body, &p) == nil {
		e.Code = firstNonEmpty(p.Code, p.Title)
		if m := firstNonEmpty(p.Detail, p.Title); m != "" {
			e.Message = m
		}
	}
	return e
}

// request is one API call. body is JSON-encoded unless raw is set.
type request struct {
	op, method, path string
	body             any
	raw              []byte
	contentType      string
	out              any // JSON target; *string for text/plain
}

// session signs in when needed and returns the token.
func (c *Client) session(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token != "" {
		return c.token, nil
	}
	tok, err := c.signIn(ctx)
	if err != nil {
		return "", err
	}
	if c.cfg.marketplace() {
		var r struct {
			UserID      int64  `json:"userId"`
			CompanyID   int64  `json:"companyId"`
			AccessToken string `json:"accessToken"`
		}
		body := map[string]string{"email": c.cfg.CustomerEmail, "password": c.cfg.CustomerPassword, "taxId": c.cfg.CustomerTaxID}
		if _, err := c.send(ctx, tok, request{op: "customerSignIn", method: http.MethodPost, path: "/api/invoice/partners/customer/signin", body: body, out: &r}); err != nil {
			return "", err
		}
		if r.AccessToken == "" {
			return "", &provider.Error{Kind: provider.KindAuth, Provider: Name, Op: "customerSignIn", Message: "no accessToken"}
		}
		tok, c.userID, c.companyID = r.AccessToken, r.UserID, r.CompanyID
	}
	c.token = tok
	return tok, nil
}

// signIn calls POST /api/auth/signin; the token is in the x-access-token
// response header.
func (c *Client) signIn(ctx context.Context) (string, error) {
	h, err := c.send(ctx, "", request{op: "signIn", method: http.MethodPost, path: "/api/auth/signin", body: map[string]string{"email": c.cfg.Email, "password": c.cfg.Password}})
	if err != nil {
		return "", err
	}
	tok := h.Get("x-access-token")
	if tok == "" {
		return "", &provider.Error{Kind: provider.KindAuth, Provider: Name, Op: "signIn", Message: "no x-access-token header"}
	}
	return tok, nil
}

func (c *Client) ids() (userID, companyID int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.userID, c.companyID
}

// call runs an authenticated request, signing in again once on HTTP 401.
func (c *Client) call(ctx context.Context, r request) error {
	for attempt := 0; ; attempt++ {
		tok, err := c.session(ctx)
		if err != nil {
			return err
		}
		_, err = c.send(ctx, tok, r)
		if attempt == 0 && provider.KindOf(err) == provider.KindAuth {
			c.mu.Lock()
			if c.token == tok {
				c.token = ""
			}
			c.mu.Unlock()
			continue
		}
		return err
	}
}

// send performs one HTTP request. The documentation names the token
// header (x-access-token) but not how to send it back, so both
// x-access-token and Authorization: Bearer are set.
func (c *Client) send(ctx context.Context, token string, r request) (http.Header, error) {
	var payload []byte
	ct := r.contentType
	switch {
	case r.raw != nil:
		payload = r.raw
	case r.body != nil:
		b, err := json.Marshal(r.body)
		if err != nil {
			return nil, &provider.Error{Kind: provider.KindInvalidRequest, Provider: Name, Op: r.op, Err: err}
		}
		payload, ct = b, "application/json"
	}
	req, err := http.NewRequestWithContext(ctx, r.method, c.base+r.path, bytes.NewReader(payload))
	if err != nil {
		return nil, &provider.Error{Kind: provider.KindInvalidRequest, Provider: Name, Op: r.op, Err: err}
	}
	if payload == nil {
		req.Body = http.NoBody
	}
	if ct != "" {
		req.Header.Set("Content-Type", ct)
	}
	req.Header.Set("Accept", "application/json, text/plain")
	if token != "" {
		req.Header.Set("x-access-token", token)
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := c.http.Do(req)
	if err != nil {
		return nil, provider.TransportError(Name, r.op, err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, maxBody+1))
	if err != nil {
		return nil, provider.TransportError(Name, r.op, err)
	}
	if len(body) > maxBody {
		return nil, &provider.Error{Kind: provider.KindTransport, Provider: Name, Op: r.op, Message: "response too large"}
	}
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return nil, apiError(r.op, res.StatusCode, body)
	}
	switch out := r.out.(type) {
	case nil:
	case *string:
		*out = strings.TrimSpace(string(body))
	default:
		if err := json.Unmarshal(body, out); err != nil {
			return nil, &provider.Error{Kind: provider.KindTransport, Provider: Name, Op: r.op, Message: "decode response", Raw: body, Err: err}
		}
	}
	return res.Header, nil
}

// download fetches a temporary document link without credentials.
func (c *Client) download(ctx context.Context, op, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return nil, &provider.Error{Kind: provider.KindTransport, Provider: Name, Op: op, Message: "bad download URL", Err: err}
	}
	res, err := c.http.Do(req)
	if err != nil {
		return nil, provider.TransportError(Name, op, err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, maxDownload+1))
	if err != nil {
		return nil, provider.TransportError(Name, op, err)
	}
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return nil, provider.HTTPError(Name, op, res.StatusCode, body)
	}
	if len(body) > maxDownload {
		return nil, &provider.Error{Kind: provider.KindTransport, Provider: Name, Op: op, Message: "download too large"}
	}
	return body, nil
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}
