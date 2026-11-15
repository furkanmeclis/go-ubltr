package nes

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
	"github.com/furkanmeclis/go-ubltr/provider/pack"
	"github.com/furkanmeclis/go-ubltr/provider/providertest"
)

const invoiceXML = `<?xml version="1.0" encoding="UTF-8"?>
<Invoice xmlns="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2"
 xmlns:cbc="urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2"
 xmlns:cac="urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2">
 <cbc:ProfileID>%PROFILE%</cbc:ProfileID>
 <cbc:ID>NES2024000000001</cbc:ID>
 <cbc:UUID>312ea9a9-85a5-40b4-a95e-1da5ba8aa4f7</cbc:UUID>
 <cbc:InvoiceTypeCode>SATIS</cbc:InvoiceTypeCode>
 <cac:AccountingCustomerParty><cac:Party><cac:PartyIdentification>
  <cbc:ID schemeID="VKN">1234567890</cbc:ID>
 </cac:PartyIdentification></cac:Party></cac:AccountingCustomerParty>
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
				if len(rt.body) > 0 && rt.body[0] == '{' || len(rt.body) > 0 && rt.body[0] == '[' {
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
	c, err := New(Config{APIKey: "test-key", Endpoint: srv.URL, SenderAlias: "urn:mail:defaultgb@nes.com.tr", HTTP: srv.Client()})
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
	p, err := provider.Open(Name, provider.Config{APIKey: "k", Test: true, Options: map[string]string{"source_app": "erp"}})
	if err != nil {
		t.Fatal(err)
	}
	c := p.(*Client)
	if c.base != TestURL || c.sourceApp != "erp" {
		t.Fatalf("base=%s sourceApp=%s", c.base, c.sourceApp)
	}
	if _, err := New(Config{}); !errors.Is(err, provider.ErrAuth) {
		t.Fatalf("missing key: %v", err)
	}
	live, _ := New(Config{APIKey: "k"})
	if live.base != LiveURL {
		t.Fatalf("live base = %s", live.base)
	}
}

func TestSendEInvoice(t *testing.T) {
	c := server(t, route{
		method: http.MethodPost, path: "/einvoice/v1/uploads/document", status: http.StatusCreated,
		body: fixture(t, "upload.json"),
		check: func(t *testing.T, r *http.Request) {
			if err := r.ParseMultipartForm(4 << 20); err != nil {
				t.Fatal(err)
			}
			want := map[string]string{
				"IsDirectSend": "true", "PreviewType": "None", "SourceApp": DefaultSourceApp,
				"AutoSaveCompany": "false", "SenderAlias": "urn:mail:defaultgb@nes.com.tr",
				"ReceiverAlias": "urn:mail:defaultpk@example.com",
			}
			for k, v := range want {
				if got := r.FormValue(k); got != v {
					t.Errorf("%s = %q, want %q", k, got, v)
				}
			}
			f, h, err := r.FormFile("File")
			if err != nil {
				t.Fatal(err)
			}
			b, _ := io.ReadAll(f)
			if h.Filename != "312ea9a9-85a5-40b4-a95e-1da5ba8aa4f7.xml" || !strings.Contains(string(b), "<cbc:UUID>") {
				t.Errorf("file %s: %q", h.Filename, b)
			}
		},
	})
	res, err := c.Send(context.Background(), &provider.Document{XML: xmlFor("TICARIFATURA"), ReceiverAlias: "urn:mail:defaultpk@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if res.UUID != "312ea9a9-85a5-40b4-a95e-1da5ba8aa4f7" || res.ID != "NES2024000000001" || res.Status != provider.StatusQueued {
		t.Fatalf("%+v", res)
	}
}

func TestSendEArchiveZipsLargeXML(t *testing.T) {
	big := xmlFor("EARSIVFATURA")
	big = append(big, []byte("<!--"+strings.Repeat(" ", MaxFileSize)+"-->")...)
	c := server(t, route{
		method: http.MethodPost, path: "/earchive/v1/uploads/document", status: http.StatusOK,
		body: fixture(t, "upload.json"),
		check: func(t *testing.T, r *http.Request) {
			if err := r.ParseMultipartForm(4 << 20); err != nil {
				t.Fatal(err)
			}
			if r.FormValue("SenderAlias") != "" {
				t.Error("SenderAlias sent for e-Arşiv")
			}
			f, h, err := r.FormFile("File")
			if err != nil {
				t.Fatal(err)
			}
			b, _ := io.ReadAll(f)
			if !strings.HasSuffix(h.Filename, ".zip") || !pack.IsZip(b) {
				t.Errorf("want zip upload, got %s", h.Filename)
			}
		},
	})
	if _, err := c.Send(context.Background(), &provider.Document{XML: big}); err != nil {
		t.Fatal(err)
	}
}

func TestSendNeedsSenderAlias(t *testing.T) {
	c, _ := New(Config{APIKey: "k"})
	_, err := c.Send(context.Background(), &provider.Document{XML: xmlFor("TEMELFATURA")})
	if !errors.Is(err, provider.ErrInvalidRequest) {
		t.Fatalf("got %v", err)
	}
}

func TestSendErrors(t *testing.T) {
	cases := []struct {
		name    string
		status  int
		fixture string
		kind    provider.ErrorKind
		code    string
		gib     int
	}{
		{"schematron", http.StatusUnprocessableEntity, "error_422.json", provider.KindValidation, "1150", 1150},
		{"exists", http.StatusUnprocessableEntity, "error_422_exists.json", provider.KindDuplicate, "422-15", 0},
		{"invalid", http.StatusBadRequest, "error_400.json", provider.KindInvalidRequest, "", 0},
		{"scope", http.StatusForbidden, "", provider.KindPermission, "", 0},
		{"auth", http.StatusUnauthorized, "", provider.KindAuth, "", 0},
		{"numbering", http.StatusConflict, "", provider.KindDuplicate, "", 0},
		{"down", http.StatusServiceUnavailable, "", provider.KindUnavailable, "", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var body []byte
			if tc.fixture != "" {
				body = fixture(t, tc.fixture)
			}
			c := server(t, route{method: http.MethodPost, path: "/einvoice/v1/uploads/document", status: tc.status, body: body})
			_, err := c.Send(context.Background(), &provider.Document{XML: xmlFor("TEMELFATURA")})
			var pe *provider.Error
			if !errors.As(err, &pe) {
				t.Fatalf("got %v", err)
			}
			if pe.Kind != tc.kind || pe.Code != tc.code || pe.GIBCode != tc.gib || pe.HTTPStatus != tc.status {
				t.Fatalf("got %+v", pe)
			}
			if tc.name == "invalid" && !strings.Contains(pe.Message, "SenderAlias") {
				t.Errorf("message %q", pe.Message)
			}
			if tc.name == "down" && !provider.IsRetryable(err) {
				t.Error("503 not retryable")
			}
		})
	}
}

func TestTransportError(t *testing.T) {
	c, _ := New(Config{APIKey: "k", Endpoint: "http://127.0.0.1:1"})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := c.Status(ctx, provider.StatusQuery{UUID: "x"})
	if k := provider.KindOf(err); k != provider.KindTransport && k != provider.KindTimeout {
		t.Fatalf("got %v", err)
	}
}

func TestStatusOutgoing(t *testing.T) {
	c := server(t, route{method: http.MethodGet, path: "/einvoice/v1/outgoing/invoices/312ea9a9-85a5-40b4-a95e-1da5ba8aa4f7", status: 200, body: fixture(t, "outgoing_detail.json")})
	res, err := c.Status(context.Background(), provider.StatusQuery{Kind: provider.EInvoice, UUID: "312ea9a9-85a5-40b4-a95e-1da5ba8aa4f7"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != provider.StatusAccepted || res.ResponseCode != "KABUL" || res.GIBCode != 1300 ||
		res.ProviderCode != "EnvelopeHasBeenTransferredToReceiverSuccessfully" || res.ID != "NES2024000000001" {
		t.Fatalf("%+v", res)
	}
	if res.UpdatedAt.IsZero() {
		t.Error("UpdatedAt not set")
	}
}

func TestStatusEArchive(t *testing.T) {
	c := server(t, route{method: http.MethodGet, path: "/earchive/v1/invoices/4517e248-a100-4514-9e9d-3101afe355ee", status: 200, body: fixture(t, "earchive_detail.json")})
	res, err := c.Status(context.Background(), provider.StatusQuery{Kind: provider.EArchive, UUID: "4517e248-a100-4514-9e9d-3101afe355ee"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != provider.StatusSent || res.ProviderCode != "Signed" {
		t.Fatalf("%+v", res)
	}
}

func TestStatusNotFound(t *testing.T) {
	c := server(t, route{method: http.MethodGet, path: "/einvoice/v1/incoming/invoices/x", status: 404})
	_, err := c.Status(context.Background(), provider.StatusQuery{Direction: provider.Inbound, UUID: "x"})
	if !errors.Is(err, provider.ErrNotFound) {
		t.Fatalf("got %v", err)
	}
}

func TestStatusMapping(t *testing.T) {
	cases := map[string]provider.Status{
		"Waiting":                             provider.StatusQueued,
		"WaitingToBeSigned":                   provider.StatusProcessing,
		"EnvelopeIsWaitingToBeProcessedByGib": provider.StatusProcessing,
		"EnvelopeSystemResponseIsBeingWaitedFromReceiver":  provider.StatusSent,
		"EnvelopeHasBeenTransferredToReceiverSuccessfully": provider.StatusDelivered,
		"ErrorHasBeenDetectedInEnvelopeByGib":              provider.StatusFailed,
		"ErrorHasBeenDetectedInEnvelopeByReceiver":         provider.StatusFailed,
		"UnknownStatus": provider.StatusUnknown,
	}
	for in, want := range cases {
		if got := OutgoingStatus(in); got != want {
			t.Errorf("%s: %s, want %s", in, got, want)
		}
	}
	if ArchiveStatus("Signed", true, "") != provider.StatusCancelled {
		t.Error("cancelled")
	}
	if ArchiveStatus("Signed", false, "Completed") != provider.StatusDelivered {
		t.Error("reported")
	}
	if ArchiveStatus("Error", false, "") != provider.StatusFailed {
		t.Error("error")
	}
}

func TestListInbox(t *testing.T) {
	c := server(t, route{
		method: http.MethodGet, path: "/einvoice/v1/incoming/invoices", status: 200, body: fixture(t, "incoming_list.json"),
		check: func(t *testing.T, r *http.Request) {
			q := r.URL.Query()
			if q.Get("sort") != "CreatedAt desc" || q.Get("page") != "1" || q.Get("pageSize") != "2" || q.Get("erpTransferred") != "False" {
				t.Errorf("query %v", q)
			}
			if q.Get("startCreateDate") != "2024-03-01T00:00:00Z" {
				t.Errorf("startCreateDate %q", q.Get("startCreateDate"))
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
	if a.UUID != "72d0ec9a-96b9-4f41-b0cb-c9598a641baa" || a.SenderVKN != "1234567890" || a.SenderName != "Örnek A.Ş." ||
		a.Payable != "1180.5" || a.IssueDate != "2024-03-01" || a.Status != provider.StatusReceived || a.GIBCode != 1300 || a.Direction != provider.Inbound {
		t.Fatalf("%+v", a)
	}
	if b.SenderName != "Ayşe Yılmaz" || b.Type != "IADE" || b.Profile != "TEMELFATURA" {
		t.Fatalf("%+v", b)
	}
}

func TestListOutboxEArchive(t *testing.T) {
	list := `{"page":1,"pageSize":10,"totalCount":1,"data":[` + string(fixture(t, "earchive_detail.json")) + `]}`
	c := server(t, route{
		method: http.MethodGet, path: "/earchive/v1/invoices", status: 200, body: []byte(list),
		check: func(t *testing.T, r *http.Request) {
			if r.URL.Query().Get("includeCanceledDocuments") != "true" {
				t.Error("includeCanceledDocuments not set")
			}
		},
	})
	page, err := c.ListOutbox(context.Background(), provider.ListQuery{Kind: provider.EArchive})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Next != "" || page.Items[0].ReceiverName != "Ali Veli" || page.Items[0].Profile != "EARSIVFATURA" {
		t.Fatalf("%+v", page)
	}
}

func TestUnsupported(t *testing.T) {
	c, _ := New(Config{APIKey: "k"})
	if _, err := c.ListInbox(context.Background(), provider.ListQuery{Kind: provider.EArchive}); !errors.Is(err, provider.ErrUnsupported) {
		t.Errorf("inbox earchive: %v", err)
	}
	if _, err := c.Status(context.Background(), provider.StatusQuery{Kind: provider.EDespatch, UUID: "x"}); !errors.Is(err, provider.ErrUnsupported) {
		t.Errorf("despatch: %v", err)
	}
	if err := c.MarkTaken(context.Background(), provider.Outbound, "x"); !errors.Is(err, provider.ErrUnsupported) {
		t.Errorf("mark outbound: %v", err)
	}
}

func TestFetch(t *testing.T) {
	c := server(t,
		route{method: http.MethodGet, path: "/einvoice/v1/incoming/invoices/u1/xml", status: 200, body: []byte("<Invoice/>")},
		route{method: http.MethodGet, path: "/earchive/v1/invoices/u2/pdf", status: 200, body: []byte("%PDF-1.4")},
		route{method: http.MethodGet, path: "/einvoice/v1/outgoing/invoices/u3/html", status: 200, body: []byte("<html></html>")},
	)
	ctx := context.Background()
	r, err := c.Fetch(ctx, provider.FetchQuery{Direction: provider.Inbound, UUID: "u1"})
	if err != nil || string(r.Data) != "<Invoice/>" || r.Format != provider.FormatUBL {
		t.Fatalf("%v %+v", err, r)
	}
	r, err = c.Fetch(ctx, provider.FetchQuery{Kind: provider.EArchive, UUID: "u2", Format: provider.FormatPDF})
	if err != nil || string(r.Data) != "%PDF-1.4" {
		t.Fatalf("%v %+v", err, r)
	}
	r, err = c.Fetch(ctx, provider.FetchQuery{UUID: "u3", Format: provider.FormatHTML})
	if err != nil || r.Format != provider.FormatHTML {
		t.Fatalf("%v %+v", err, r)
	}
}

func TestRespond(t *testing.T) {
	c := server(t, route{
		method: http.MethodPost, path: "/einvoice/v1/incoming/invoices/72d0ec9a-96b9-4f41-b0cb-c9598a641baa/documentAnswer", status: 201,
		body: []byte(`{"documentAnswer":"Rejected","answerNote":"Hatalı tutar","outgoingStatus":"Waiting","recordStatus":"Waiting","errorDescription":null,"outgoingAnswerEnvelope":null}`),
		check: func(t *testing.T, r *http.Request) {
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body["incomingInvoiceAnswerParameter"] != "RED" || body["answerNote"] != "Hatalı tutar" {
				t.Errorf("body %v", body)
			}
		},
	})
	if err := c.Respond(context.Background(), provider.Response{UUID: "72d0ec9a-96b9-4f41-b0cb-c9598a641baa", Reason: "Hatalı tutar"}); err != nil {
		t.Fatal(err)
	}
}

func TestRespondAlreadyAnswered(t *testing.T) {
	c := server(t, route{method: http.MethodPost, path: "/einvoice/v1/incoming/invoices/u/documentAnswer", status: 409})
	if err := c.Respond(context.Background(), provider.Response{UUID: "u", Accept: true}); !errors.Is(err, provider.ErrDuplicate) {
		t.Fatalf("got %v", err)
	}
}

func TestCancelEArchive(t *testing.T) {
	c := server(t, route{
		method: http.MethodPost, path: "/earchive/v1/invoices/cancel", status: 200,
		check: func(t *testing.T, r *http.Request) {
			b, _ := io.ReadAll(r.Body)
			if string(b) != `{"uuids":["4517e248-a100-4514-9e9d-3101afe355ee"]}` {
				t.Errorf("body %s", b)
			}
		},
	})
	if err := c.CancelEArchive(context.Background(), "4517e248-a100-4514-9e9d-3101afe355ee", "ignored"); err != nil {
		t.Fatal(err)
	}
}

func TestCancelEArchiveAlreadyCancelled(t *testing.T) {
	c := server(t, route{method: http.MethodPost, path: "/earchive/v1/invoices/cancel", status: 422, body: fixture(t, "error_422_canceled.json")})
	err := c.CancelEArchive(context.Background(), "u", "")
	if !errors.Is(err, provider.ErrInvalidState) {
		t.Fatalf("got %v", err)
	}
}

func TestCheckUser(t *testing.T) {
	c := server(t,
		route{method: http.MethodGet, path: "/einvoice/v1/users/1234567890/All", status: 200, body: fixture(t, "user.json")},
		route{method: http.MethodGet, path: "/einvoice/v1/users/9999999999/All", status: 404},
	)
	u, err := c.CheckUser(context.Background(), "1234567890")
	if err != nil {
		t.Fatal(err)
	}
	if !u.Registered || u.Title != "Örnek A.Ş." || len(u.Aliases) != 2 || u.Aliases[0].Type != "PK" || u.Aliases[1].Type != "GB" {
		t.Fatalf("%+v", u)
	}
	u, err = c.CheckUser(context.Background(), "9999999999")
	if err != nil || u.Registered {
		t.Fatalf("%v %+v", err, u)
	}
}

func TestMarkTaken(t *testing.T) {
	c := server(t, route{
		method: http.MethodPut, path: "/einvoice/v1/incoming/invoices/bulk/Transferred", status: 200,
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
	c := server(t, route{method: http.MethodGet, path: "/general/v1/management/creditsummary", status: 200, body: fixture(t, "credit_summary.json")})
	ci, err := c.Credits(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if ci.Remaining != 700 || ci.Used != 250 {
		t.Fatalf("%+v", ci)
	}
}
