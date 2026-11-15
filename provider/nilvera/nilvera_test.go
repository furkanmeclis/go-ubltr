package nilvera

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
 xmlns:cbc="urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2"
 xmlns:cac="urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2">
 <cbc:ProfileID>%PROFILE%</cbc:ProfileID>
 <cbc:ID>NIL2024000000001</cbc:ID>
 <cbc:UUID>e1b40d5b-24b3-4f79-8735-a01ae5577de7</cbc:UUID>
 <cbc:InvoiceTypeCode>SATIS</cbc:InvoiceTypeCode>
</Invoice>`

func xmlFor(profile string) []byte {
	return []byte(strings.ReplaceAll(invoiceXML, "%PROFILE%", profile))
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
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization = %q", got)
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
}

func TestRegistry(t *testing.T) {
	p, err := provider.Open(Name, provider.Config{APIKey: "k", Test: true})
	if err != nil {
		t.Fatal(err)
	}
	if p.(*Client).base != TestURL {
		t.Fatalf("base = %s", p.(*Client).base)
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
		method: http.MethodPost, path: "/einvoice/Send/Xml", status: 200, body: fixture(t, "send.json"),
		check: func(t *testing.T, r *http.Request) {
			if got := r.URL.Query().Get("Alias"); got != "urn:mail:defaultpk@example.com" {
				t.Errorf("Alias = %q", got)
			}
			f, h, err := r.FormFile("file")
			if err != nil {
				t.Fatal(err)
			}
			b, _ := io.ReadAll(f)
			if h.Filename != "e1b40d5b-24b3-4f79-8735-a01ae5577de7.xml" || !strings.Contains(string(b), "<cbc:UUID>") {
				t.Errorf("file %s: %q", h.Filename, b)
			}
		},
	})
	res, err := c.Send(context.Background(), &provider.Document{XML: xmlFor("TICARIFATURA"), ReceiverAlias: "urn:mail:defaultpk@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if res.UUID != "e1b40d5b-24b3-4f79-8735-a01ae5577de7" || res.ID != "NIL2024000000001" || res.Status != provider.StatusQueued {
		t.Fatalf("%+v", res)
	}
}

func TestSendEArchive(t *testing.T) {
	c := server(t, route{
		method: http.MethodPost, path: "/earchive/Send/Xml", status: 200, body: fixture(t, "send.json"),
		check: func(t *testing.T, r *http.Request) {
			if r.URL.Query().Has("Alias") {
				t.Error("Alias sent for e-Arşiv")
			}
		},
	})
	if _, err := c.Send(context.Background(), &provider.Document{XML: xmlFor("EARSIVFATURA"), ReceiverAlias: "x"}); err != nil {
		t.Fatal(err)
	}
}

func TestErrors(t *testing.T) {
	cases := []struct {
		name    string
		status  int
		fixture string
		kind    provider.ErrorKind
		code    string
	}{
		{"bad request", 400, "error_400.json", provider.KindInvalidRequest, "400"},
		{"not found", 404, "error_404.json", provider.KindNotFound, "3006"},
		{"duplicate", 409, "error_409.json", provider.KindDuplicate, "1001"},
		{"credits", 422, "error_422.json", provider.KindQuotaExceeded, "2000"},
		{"unauthorized", 401, "", provider.KindAuth, ""},
		{"forbidden", 403, "", provider.KindPermission, ""},
		{"server", 502, "", provider.KindUnavailable, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var body []byte
			if tc.fixture != "" {
				body = fixture(t, tc.fixture)
			}
			c := server(t, route{method: http.MethodPost, path: "/einvoice/Send/Xml", status: tc.status, body: body})
			_, err := c.Send(context.Background(), &provider.Document{XML: xmlFor("TEMELFATURA")})
			var pe *provider.Error
			if !errors.As(err, &pe) {
				t.Fatalf("got %v", err)
			}
			if pe.Kind != tc.kind || pe.Code != tc.code || pe.HTTPStatus != tc.status || pe.GIBCode != 0 {
				t.Fatalf("got %+v", pe)
			}
			if tc.fixture != "" && pe.Message == "" {
				t.Error("empty message")
			}
		})
	}
}

func TestUnknownCodeKeepsHTTPKind(t *testing.T) {
	body := []byte(`{"Message":"HATALI ISTEK","Errors":[{"Code":2004,"Description":"Kayıt Başarısız.","Detail":""}]}`)
	c := server(t, route{method: http.MethodPost, path: "/einvoice/Send/Xml", status: 422, body: body})
	_, err := c.Send(context.Background(), &provider.Document{XML: xmlFor("TEMELFATURA")})
	if !errors.Is(err, provider.ErrInvalidRequest) {
		t.Fatalf("got %v", err)
	}
}

func TestStatusSale(t *testing.T) {
	c := server(t, route{method: http.MethodGet, path: "/einvoice/Sale/u1/Status", status: 200, body: fixture(t, "sale_status.json")})
	res, err := c.Status(context.Background(), provider.StatusQuery{UUID: "u1"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != provider.StatusDelivered || res.GIBCode != 1300 || res.ProviderCode != "succeed" || res.UpdatedAt.IsZero() {
		t.Fatalf("%+v", res)
	}
}

func TestStatusPurchaseAnswered(t *testing.T) {
	body := strings.Replace(string(fixture(t, "sale_status.json")), "waitingForApproval", "rejected", 1)
	c := server(t, route{method: http.MethodGet, path: "/einvoice/Purchase/u2/Status", status: 200, body: []byte(body)})
	res, err := c.Status(context.Background(), provider.StatusQuery{Direction: provider.Inbound, UUID: "u2"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != provider.StatusRejected || res.ResponseCode != "RED" {
		t.Fatalf("%+v", res)
	}
}

func TestStatusEArchive(t *testing.T) {
	c := server(t, route{method: http.MethodGet, path: "/earchive/Invoices/u3/Status", status: 200, body: fixture(t, "earchive_status.json")})
	res, err := c.Status(context.Background(), provider.StatusQuery{Kind: provider.EArchive, UUID: "u3"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != provider.StatusDelivered || res.Message != "Başarılı" {
		t.Fatalf("%+v", res)
	}
}

func TestStatusMapping(t *testing.T) {
	cases := []struct {
		code string
		gib  int
		want provider.Status
	}{
		{"waiting", 0, provider.StatusProcessing},
		{"waiting", 1000, provider.StatusQueued},
		{"succeed", 0, provider.StatusSent},
		{"succeed", 1220, provider.StatusSent},
		{"succeed", 1300, provider.StatusDelivered},
		{"succeed", 1230, provider.StatusFailed},
		{"error", 1300, provider.StatusFailed},
		{"unknown", 0, provider.StatusUnknown},
	}
	for _, tc := range cases {
		if got := InvoiceStatus(tc.code, tc.gib); got != tc.want {
			t.Errorf("%s/%d: %s, want %s", tc.code, tc.gib, got, tc.want)
		}
	}
	if ArchiveStatus("succeed", "NotReported", true) != provider.StatusCancelled {
		t.Error("cancelled")
	}
	if ArchiveStatus("succeed", "NotReported", false) != provider.StatusSent {
		t.Error("not reported")
	}
	if s, _, _ := answerStatus("documentAnsweredAutomatically"); s != provider.StatusAccepted {
		t.Error("auto answer")
	}
}

func TestListInbox(t *testing.T) {
	c := server(t, route{
		method: http.MethodGet, path: "/einvoice/Purchase", status: 200, body: fixture(t, "purchase_list.json"),
		check: func(t *testing.T, r *http.Request) {
			q := r.URL.Query()
			if q.Get("Page") != "1" || q.Get("PageSize") != "1" || q.Get("IsTransfer") != "false" ||
				q.Get("DateFilterType") != "CreatedDate" || q.Get("StartDate") != "2024-03-01T00:00:00Z" {
				t.Errorf("query %v", q)
			}
		},
	})
	page, err := c.ListInbox(context.Background(), provider.ListQuery{OnlyNew: true, Limit: 1, From: time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Next != "2" {
		t.Fatalf("%+v", page)
	}
	it := page.Items[0]
	if it.UUID != "72d0ec9a-96b9-4f41-b0cb-c9598a641baa" || it.SenderVKN != "1234567890" || it.ReceiverVKN != "9876543210" ||
		it.Payable != "1180.5" || it.IssueDate != "2024-03-01" || it.Status != provider.StatusReceived || it.Profile != "TICARIFATURA" {
		t.Fatalf("%+v", it)
	}
}

func TestListOutboxEArchive(t *testing.T) {
	body := `{"Page":1,"PageSize":50,"TotalCount":1,"TotalPages":1,"Content":[{"UUID":"u","InvoiceNumber":"EAR2024000000001","SendType":"ELEKTRONIK","IssueDate":"2024-03-01T00:00:00","TotalAmount":120,"PayableAmount":120,"CurrencyCode":"TRY","ReceiverTaxNumber":"11111111111","ReceiverName":"Ali Veli","CreatedDate":"2024-03-01T10:00:00","InvoiceType":"SATIS","StatusCode":"succeed","StatusDetail":null,"IsCancel":true,"IsReport":false}]}`
	c := server(t, route{method: http.MethodGet, path: "/earchive/Invoices", status: 200, body: []byte(body)})
	page, err := c.ListOutbox(context.Background(), provider.ListQuery{Kind: provider.EArchive})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Next != "" || page.Items[0].Status != provider.StatusCancelled || page.Items[0].ReceiverName != "Ali Veli" {
		t.Fatalf("%+v", page)
	}
}

func TestUnsupported(t *testing.T) {
	c, _ := New(Config{APIKey: "k"})
	if _, err := c.ListInbox(context.Background(), provider.ListQuery{Kind: provider.EArchive}); !errors.Is(err, provider.ErrUnsupported) {
		t.Errorf("inbox earchive: %v", err)
	}
	if err := c.MarkTaken(context.Background(), provider.Outbound, "x"); !errors.Is(err, provider.ErrUnsupported) {
		t.Errorf("outbound: %v", err)
	}
	if _, err := c.Fetch(context.Background(), provider.FetchQuery{UUID: "x", Format: "zip"}); !errors.Is(err, provider.ErrUnsupported) {
		t.Errorf("format: %v", err)
	}
}

func TestFetch(t *testing.T) {
	c := server(t,
		route{method: http.MethodGet, path: "/einvoice/Purchase/u1/xml", status: 200, body: []byte("<Invoice/>")},
		route{method: http.MethodGet, path: "/einvoice/Sale/u2/pdf", status: 200, body: []byte("%PDF-1.4")},
		route{method: http.MethodGet, path: "/earchive/Invoices/u3/html", status: 200, body: []byte("<html></html>")},
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
		method: http.MethodPost, path: "/einvoice/Purchase/SendAnswer", status: 200, body: []byte(`"OK"`),
		check: func(t *testing.T, r *http.Request) {
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body["UUID"] != "u" || body["AnswerCode"] != "approved" || body["RejectNote"] != "" {
				t.Errorf("body %v", body)
			}
		},
	})
	if err := c.Respond(context.Background(), provider.Response{UUID: "u", Accept: true, Reason: "ignored"}); err != nil {
		t.Fatal(err)
	}
}

func TestRespondAlreadyAnswered(t *testing.T) {
	body := []byte(`{"Message":"HATALI ISTEK","Errors":[{"Code":1007,"Description":"E-Fatura Daha Önce Cevaplanmış.","Detail":""}]}`)
	c := server(t, route{method: http.MethodPost, path: "/einvoice/Purchase/SendAnswer", status: 409, body: body})
	if err := c.Respond(context.Background(), provider.Response{UUID: "u"}); !errors.Is(err, provider.ErrDuplicate) {
		t.Fatalf("got %v", err)
	}
}

func TestCancelEArchive(t *testing.T) {
	c := server(t, route{method: http.MethodPut, path: "/earchive/Invoices/u/Cancel", status: 200, body: []byte(`[]`)})
	if err := c.CancelEArchive(context.Background(), "u", ""); err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"Message":"HATALI ISTEK","Errors":[{"Code":2026,"Description":"E-Arşiv Fatura Daha Önce İptal Edilmiştir.","Detail":""}]}`)
	c = server(t, route{method: http.MethodPut, path: "/earchive/Invoices/u/Cancel", status: 422, body: body})
	if err := c.CancelEArchive(context.Background(), "u", ""); !errors.Is(err, provider.ErrInvalidState) {
		t.Fatalf("got %v", err)
	}
}

func TestCheckUser(t *testing.T) {
	c := server(t,
		route{
			method: http.MethodGet, path: "/general/GlobalCompany/Check/TaxNumber/1234567890", status: 200, body: fixture(t, "global_company.json"),
			check: func(t *testing.T, r *http.Request) {
				if r.URL.Query().Get("globalUserType") != "Invoice" {
					t.Error("globalUserType not set")
				}
			},
		},
		route{method: http.MethodGet, path: "/general/GlobalCompany/Check/TaxNumber/9999999999", status: 200, body: []byte(`[]`)},
	)
	u, err := c.CheckUser(context.Background(), "1234567890")
	if err != nil {
		t.Fatal(err)
	}
	if !u.Registered || u.Title != "Örnek A.Ş." || len(u.Aliases) != 1 || u.Aliases[0].Name != "urn:mail:defaultpk@example.com" {
		t.Fatalf("%+v", u)
	}
	u, err = c.CheckUser(context.Background(), "9999999999")
	if err != nil || u.Registered {
		t.Fatalf("%v %+v", err, u)
	}
}

func TestMarkTaken(t *testing.T) {
	c := server(t, route{
		method: http.MethodPut, path: "/einvoice/Purchase/Operation/Transferred", status: 200, body: []byte(`[]`),
		check: func(t *testing.T, r *http.Request) {
			b, _ := io.ReadAll(r.Body)
			if string(b) != `["a","b"]` {
				t.Errorf("body %s", b)
			}
		},
	})
	if err := c.MarkTaken(context.Background(), provider.Inbound, "a", "b"); err != nil {
		t.Fatal(err)
	}
}

func TestCredits(t *testing.T) {
	c := server(t, route{method: http.MethodGet, path: "/general/Credits", status: 200, body: fixture(t, "credits.json")})
	ci, err := c.Credits(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if ci.Remaining != 840 || ci.Used != 260 {
		t.Fatalf("%+v", ci)
	}
}

func TestTransportError(t *testing.T) {
	c, _ := New(Config{APIKey: "k", Endpoint: "http://127.0.0.1:1"})
	_, err := c.Credits(context.Background())
	if k := provider.KindOf(err); k != provider.KindTransport && k != provider.KindTimeout {
		t.Fatalf("got %v", err)
	}
}
