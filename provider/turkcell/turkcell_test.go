package turkcell

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/go-ubltr/provider"
	"github.com/furkanmeclis/go-ubltr/provider/providertest"
)

const invoiceXML = `<?xml version="1.0" encoding="UTF-8"?>
<Invoice xmlns="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2"
 xmlns:cbc="urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2">
 <cbc:ProfileID>%PROFILE%</cbc:ProfileID>
 %ID%
 <cbc:UUID>9758339e-eb0b-454c-8c2f-18059f944a3f</cbc:UUID>
 <cbc:InvoiceTypeCode>SATIS</cbc:InvoiceTypeCode>
</Invoice>`

func xmlFor(profile, id string) []byte {
	s := strings.ReplaceAll(invoiceXML, "%PROFILE%", profile)
	if id != "" {
		id = "<cbc:ID>" + id + "</cbc:ID>"
	}
	return []byte(strings.ReplaceAll(s, "%ID%", id))
}

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

type route struct {
	method, path string
	status       int
	body         []byte
	check        func(t *testing.T, r *http.Request)
}

func server(t *testing.T, routes ...route) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Api-Key"); got != "test-key" {
			t.Errorf("x-api-key = %q", got)
		}
		for _, rt := range routes {
			if r.Method == rt.method && r.URL.Path == rt.path {
				if rt.check != nil {
					rt.check(t, r)
				}
				if len(rt.body) > 0 && (rt.body[0] == '{' || rt.body[0] == '[') {
					w.Header().Set("Content-Type", "application/json")
				}
				w.WriteHeader(rt.status)
				_, _ = w.Write(rt.body)
				return
			}
		}
		t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)
	c, err := New(Config{APIKey: "test-key", Endpoint: srv.URL, HTTP: srv.Client()})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestConformance(t *testing.T) {
	c, err := New(Config{APIKey: "k"})
	if err != nil {
		t.Fatal(err)
	}
	providertest.CheckCapabilities(t, c)
	providertest.CheckCodeTable(t, Codes)
	if provider.Supports(c, provider.EInvoice, provider.OpCheckUser) || provider.Supports(c, provider.EInvoice, provider.OpListOutbox) {
		t.Error("undocumented operation declared")
	}
}

func TestRegistry(t *testing.T) {
	p, err := provider.Open(Name, provider.Config{APIKey: "k", Test: true, Options: map[string]string{"standard_xslt": "true"}})
	if err != nil {
		t.Fatal(err)
	}
	c := p.(*Client)
	if c.base != TestURL || !c.standard {
		t.Fatalf("%+v", c)
	}
	live, _ := New(Config{APIKey: "k"})
	if live.base != LiveURL {
		t.Fatalf("live base = %s", live.base)
	}
	if _, err := New(Config{}); !errors.Is(err, provider.ErrAuth) {
		t.Fatalf("missing key: %v", err)
	}
}

func TestSendEInvoice(t *testing.T) {
	c := server(t, route{
		method: http.MethodPost, path: "/v2/outboxinvoice", status: 200, body: fixture(t, "create.json"),
		check: func(t *testing.T, r *http.Request) {
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Fatal(err)
			}
			want := map[string]string{"appType": "1", "status": "20", "useManualInvoiceId": "false", "targetAlias": "urn:mail:defaultpk@example.com"}
			for k, v := range want {
				if got := r.FormValue(k); got != v {
					t.Errorf("%s = %q, want %q", k, got, v)
				}
			}
			f, h, err := r.FormFile("invoiceFile")
			if err != nil {
				t.Fatal(err)
			}
			b, _ := io.ReadAll(f)
			if h.Filename != "9758339e-eb0b-454c-8c2f-18059f944a3f.xml" || !strings.Contains(string(b), "<cbc:UUID>") {
				t.Errorf("file %s", h.Filename)
			}
		},
	})
	res, err := c.Send(context.Background(), &provider.Document{XML: xmlFor("TICARIFATURA", ""), ReceiverAlias: "urn:mail:defaultpk@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if res.UUID != "9758339e-eb0b-454c-8c2f-18059f944a3f" || res.ID != "EPA2024000000001" || res.Status != provider.StatusQueued {
		t.Fatalf("%+v", res)
	}
}

func TestSendEArchiveWithEmailAndNumber(t *testing.T) {
	c := server(t, route{
		method: http.MethodPost, path: "/v2/earchive", status: 200, body: fixture(t, "create.json"),
		check: func(t *testing.T, r *http.Request) {
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Fatal(err)
			}
			want := map[string]string{"status": "20", "useManualInvoiceId": "true", "sendEMail": "true", "eMailAddress": "a@example.com", "appType": "", "targetAlias": ""}
			for k, v := range want {
				if got := r.FormValue(k); got != v {
					t.Errorf("%s = %q, want %q", k, got, v)
				}
			}
		},
	})
	if _, err := c.Send(context.Background(), &provider.Document{XML: xmlFor("EARSIVFATURA", "EAR2024000000001"), Email: "a@example.com"}); err != nil {
		t.Fatal(err)
	}
}

func TestErrors(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   []byte
		kind   provider.ErrorKind
		msg    string
	}{
		{"validation", 422, fixture(t, "error_422.json"), provider.KindValidation, "birden fazla etiketi"},
		{"unauthorized", 401, nil, provider.KindAuth, ""},
		{"forbidden", 403, nil, provider.KindPermission, ""},
		{"not found", 404, nil, provider.KindNotFound, ""},
		{"bad request", 400, nil, provider.KindInvalidRequest, ""},
		{"rate limit", 429, nil, provider.KindRateLimit, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := server(t, route{method: http.MethodPost, path: "/v2/outboxinvoice", status: tc.status, body: tc.body})
			_, err := c.Send(context.Background(), &provider.Document{XML: xmlFor("TEMELFATURA", "")})
			var pe *provider.Error
			if !errors.As(err, &pe) {
				t.Fatalf("got %v", err)
			}
			if pe.Kind != tc.kind || pe.HTTPStatus != tc.status || !strings.Contains(pe.Message, tc.msg) {
				t.Fatalf("got %+v", pe)
			}
			if tc.status == 429 && !provider.IsRetryable(err) {
				t.Error("429 not retryable")
			}
		})
	}
}

func TestStatusOutbox(t *testing.T) {
	c := server(t, route{method: http.MethodGet, path: "/v2/outboxinvoice/9758339e-eb0b-454c-8c2f-18059f944a3f/status", status: 200, body: fixture(t, "outbox_status.json")})
	res, err := c.Status(context.Background(), provider.StatusQuery{UUID: "9758339e-eb0b-454c-8c2f-18059f944a3f"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != provider.StatusDelivered || res.GIBCode != 1300 || res.ProviderCode != "70" || res.ID != "EPA2024000000001" || res.Message != "Başarıyla Tamamlandı" {
		t.Fatalf("%+v", res)
	}
}

func TestStatusOutboxEnvelopeFailure(t *testing.T) {
	body := `{"id":"u","invoiceNumber":"EPA2024000000001","status":50,"message":null,"envelopeId":"e","envelopeStatus":1230,"envelopeMessage":"Hedeften Sistem Yanıtı Başarısız Geldi"}`
	c := server(t, route{method: http.MethodGet, path: "/v2/outboxinvoice/u/status", status: 200, body: []byte(body)})
	res, err := c.Status(context.Background(), provider.StatusQuery{UUID: "u"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != provider.StatusFailed || res.GIBCode != 1230 {
		t.Fatalf("%+v", res)
	}
}

func TestStatusInbox(t *testing.T) {
	body := `{"Id":"u","InvoiceNumber":"ABC2024000000001","Status":"65"}`
	c := server(t, route{method: http.MethodGet, path: "/v2/inboxinvoice/u/status", status: 200, body: []byte(body)})
	res, err := c.Status(context.Background(), provider.StatusQuery{Direction: provider.Inbound, UUID: "u"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != provider.StatusAccepted || res.ResponseCode != "KABUL" || res.ID != "ABC2024000000001" {
		t.Fatalf("%+v", res)
	}
}

func TestStatusEArchive(t *testing.T) {
	body := `{"id":"u","invoiceNumber":"EAR2024000000001","status":100,"message":null}`
	c := server(t, route{method: http.MethodGet, path: "/v2/earchive/u/status", status: 200, body: []byte(body)})
	res, err := c.Status(context.Background(), provider.StatusQuery{Kind: provider.EArchive, UUID: "u"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != provider.StatusCancelled {
		t.Fatalf("%+v", res)
	}
}

func TestStatusMapping(t *testing.T) {
	out := map[int]provider.Status{0: provider.StatusDraft, 20: provider.StatusQueued, 30: provider.StatusProcessing, 40: provider.StatusFailed,
		50: provider.StatusSent, 60: provider.StatusDelivered, 65: provider.StatusAccepted, 80: provider.StatusRejected, 99: provider.StatusCancelled, 7: provider.StatusUnknown}
	for code, want := range out {
		if got := OutboxStatus(code); got != want {
			t.Errorf("outbox %d: %s, want %s", code, got, want)
		}
	}
	in := map[int]provider.Status{70: provider.StatusReceived, 60: provider.StatusReceived, 85: provider.StatusRejected, 99: provider.StatusCancelled}
	for code, want := range in {
		if got := InboxStatus(code); got != want {
			t.Errorf("inbox %d: %s, want %s", code, got, want)
		}
	}
}

func TestListInbox(t *testing.T) {
	c := server(t, route{
		method: http.MethodGet, path: "/v1/inboxinvoice/list", status: 200, body: fixture(t, "inbox_list.json"),
		check: func(t *testing.T, r *http.Request) {
			q := r.URL.Query()
			if q.Get("pageIndex") != "1" || q.Get("pageSize") != "2" || q.Get("isNew") != "true" || q.Get("startDate") != "2024-03-01 00:00:00" {
				t.Errorf("query %v", q)
			}
		},
	})
	page, err := c.ListInbox(context.Background(), provider.ListQuery{OnlyNew: true, Limit: 2, From: time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 2 || page.Next != "2" {
		t.Fatalf("%+v", page)
	}
	a, b := page.Items[0], page.Items[1]
	if a.UUID != "384ed203-22ab-4623-995b-4824ff1c6e30" || a.Profile != "TICARIFATURA" || a.Type != "SATIS" || a.SenderVKN != "1234567890" ||
		a.Payable != "1180.5" || a.IssueDate != "2024-03-01" || a.GIBCode != 1300 || a.Status != provider.StatusReceived || a.CreatedAt.IsZero() {
		t.Fatalf("%+v", a)
	}
	if b.Status != provider.StatusRejected || b.Type != "IADE" {
		t.Fatalf("%+v", b)
	}
}

func TestFetch(t *testing.T) {
	c := server(t,
		route{method: http.MethodGet, path: "/v2/inboxinvoice/u1/ubl", status: 200, body: []byte("<Invoice/>")},
		route{method: http.MethodGet, path: "/v2/outboxinvoice/u2/pdf/false", status: 200, body: []byte("%PDF-1.4")},
		route{method: http.MethodGet, path: "/v2/earchive/u3/html/false", status: 200, body: []byte("<html></html>")},
	)
	ctx := context.Background()
	if r, err := c.Fetch(ctx, provider.FetchQuery{Direction: provider.Inbound, UUID: "u1"}); err != nil || string(r.Data) != "<Invoice/>" {
		t.Fatalf("%v %+v", err, r)
	}
	if r, err := c.Fetch(ctx, provider.FetchQuery{UUID: "u2", Format: provider.FormatPDF}); err != nil || string(r.Data) != "%PDF-1.4" {
		t.Fatalf("%v %+v", err, r)
	}
	if r, err := c.Fetch(ctx, provider.FetchQuery{Kind: provider.EArchive, UUID: "u3", Format: provider.FormatHTML}); err != nil || r.Format != provider.FormatHTML {
		t.Fatalf("%v %+v", err, r)
	}
}

func TestRespond(t *testing.T) {
	c := server(t, route{
		method: http.MethodPost, path: "/v1/invoiceresponse", status: 200,
		check: func(t *testing.T, r *http.Request) {
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body["InvoiceId"] != "362fa960-1d03-4a37-8894-68d6a2566428" || body["Status"] != float64(1) || body["Reason"] != "Kabul" {
				t.Errorf("body %v", body)
			}
		},
	})
	if err := c.Respond(context.Background(), provider.Response{UUID: "362fa960-1d03-4a37-8894-68d6a2566428", Accept: true, Reason: "Kabul"}); err != nil {
		t.Fatal(err)
	}
}

func TestCancelEArchive(t *testing.T) {
	c := server(t, route{
		method: http.MethodPut, path: "/v1/earchive/cancelinvoice", status: 200,
		check: func(t *testing.T, r *http.Request) {
			b, _ := io.ReadAll(r.Body)
			if string(b) != `["3b753375-1f84-4c50-9075-2279806849ae"]` {
				t.Errorf("body %s", b)
			}
		},
	})
	if err := c.CancelEArchive(context.Background(), "3b753375-1f84-4c50-9075-2279806849ae", ""); err != nil {
		t.Fatal(err)
	}
}

func TestMarkTaken(t *testing.T) {
	c := server(t, route{
		method: http.MethodPut, path: "/v1/inboxinvoice/updateisnew", status: 200,
		check: func(t *testing.T, r *http.Request) {
			b, _ := io.ReadAll(r.Body)
			if string(b) != `[{"InvoiceId":"a","IsNew":false},{"InvoiceId":"b","IsNew":false}]` {
				t.Errorf("body %s", b)
			}
		},
	})
	if err := c.MarkTaken(context.Background(), provider.Inbound, "a", "b"); err != nil {
		t.Fatal(err)
	}
	if err := c.MarkTaken(context.Background(), provider.Outbound, "a"); !errors.Is(err, provider.ErrUnsupported) {
		t.Fatalf("outbound: %v", err)
	}
}

func TestUnsupportedKinds(t *testing.T) {
	c, _ := New(Config{APIKey: "k"})
	if _, err := c.ListInbox(context.Background(), provider.ListQuery{Kind: provider.EArchive}); !errors.Is(err, provider.ErrUnsupported) {
		t.Errorf("got %v", err)
	}
	if _, err := c.Send(context.Background(), &provider.Document{XML: []byte(`<DespatchAdvice xmlns="urn:oasis:names:specification:ubl:schema:xsd:DespatchAdvice-2"/>`)}); !errors.Is(err, provider.ErrUnsupported) {
		t.Errorf("got %v", err)
	}
}
