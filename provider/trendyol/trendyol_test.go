package trendyol_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/furkanmeclis/go-ubltr/provider"
	"github.com/furkanmeclis/go-ubltr/provider/providertest"
	"github.com/furkanmeclis/go-ubltr/provider/trendyol"
)

const invoiceXML = `<?xml version="1.0" encoding="UTF-8"?>
<Invoice xmlns="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2"
 xmlns:cac="urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2"
 xmlns:cbc="urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2">
 <cbc:ProfileID>TICARIFATURA</cbc:ProfileID>
 <cbc:ID>TYE2026000000001</cbc:ID>
 <cbc:UUID>3f2b1d0e-1111-4222-8333-944455556666</cbc:UUID>
 <cbc:InvoiceTypeCode>SATIS</cbc:InvoiceTypeCode>
 <cac:AccountingCustomerParty><cac:Party><cac:PartyIdentification><cbc:ID schemeID="VKN">2222222222</cbc:ID></cac:PartyIdentification></cac:Party></cac:AccountingCustomerParty>
</Invoice>`

type call struct {
	Method, Path string
	Header       http.Header
	Body         []byte
}

type fake struct {
	mu     sync.Mutex
	calls  []call
	routes map[string]func(w http.ResponseWriter, c call)
	logins int
}

func (f *fake) last(path string) call {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := len(f.calls) - 1; i >= 0; i-- {
		if f.calls[i].Path == path {
			return f.calls[i]
		}
	}
	return call{}
}

func jsonReply(v string) func(http.ResponseWriter, call) {
	return func(w http.ResponseWriter, _ call) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, v)
	}
}

func newServer(t *testing.T, routes map[string]func(http.ResponseWriter, call)) (*fake, *httptest.Server) {
	f := &fake{routes: routes}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		c := call{Method: r.Method, Path: r.URL.Path, Header: r.Header.Clone(), Body: body}
		f.mu.Lock()
		f.calls = append(f.calls, c)
		f.mu.Unlock()
		switch r.URL.Path {
		case "/api/auth/signin":
			var in map[string]string
			_ = json.Unmarshal(body, &in)
			if in["email"] != "api@firma.com" || in["password"] != "pw" {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			f.mu.Lock()
			f.logins++
			n := f.logins
			f.mu.Unlock()
			w.Header().Set("x-access-token", "tok"+string(rune('0'+n)))
			w.Header().Set("x-refresh-token", "refresh")
			return
		case "/dl/file":
			if r.Header.Get("Authorization") != "" {
				t.Error("download link must not carry credentials")
			}
			_, _ = io.WriteString(w, "%PDF-1.4")
			return
		}
		if r.Header.Get("x-access-token") == "" || !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		h, ok := f.routes[r.URL.Path]
		if !ok {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		h(w, c)
	}))
	t.Cleanup(srv.Close)
	return f, srv
}

func newClient(t *testing.T, url string) *trendyol.Client {
	t.Helper()
	c, err := trendyol.New(trendyol.Config{Email: "api@firma.com", Password: "pw", UserID: 11, CompanyID: 22, BaseURL: url})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestCapabilitiesAndCodes(t *testing.T) {
	c := newClient(t, "http://127.0.0.1:1")
	providertest.CheckCapabilities(t, c)
	providertest.CheckCodeTable(t, trendyol.Codes)
	if _, err := trendyol.New(trendyol.Config{Email: "a", Password: "b"}); err == nil {
		t.Fatal("API user without UserID/CompanyID must fail")
	}
	p, err := provider.Open("trendyol", provider.Config{Username: "a", Password: "b", Test: true, Options: map[string]string{"user_id": "1", "company_id": "2"}})
	if err != nil || p.Name() != "trendyol" {
		t.Fatalf("Open: %v", err)
	}
}

func TestSendEInvoiceByFile(t *testing.T) {
	f, srv := newServer(t, map[string]func(http.ResponseWriter, call){
		"/api/invoice/documents/outgoing-einvoice-by-file": jsonReply(`{"invoiceUuid":"3f2b1d0e-1111-4222-8333-944455556666","invoiceId":"TYE2026000000001","status":"30","gibStatusCode":1000,"envelopeId":"7b99499b-7cf5-4a69-a407-22dab3c43d1e","targetAlias":"urn:mail:defaultpk@x.com"}`),
	})
	c := newClient(t, srv.URL)
	res, err := c.Send(context.Background(), &provider.Document{XML: []byte(invoiceXML), ReceiverAlias: "urn:mail:defaultpk@x.com"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != provider.StatusQueued || res.ProviderCode != "30" || res.EnvelopeUUID == "" || res.GIBCode != 1000 {
		t.Fatalf("result %+v", res)
	}
	cl := f.last("/api/invoice/documents/outgoing-einvoice-by-file")
	if cl.Header.Get("x-access-token") != "tok1" || cl.Header.Get("Authorization") != "Bearer tok1" {
		t.Fatalf("auth headers %v", cl.Header)
	}
	_, params, err := mime.ParseMediaType(cl.Header.Get("Content-Type"))
	if err != nil {
		t.Fatal(err)
	}
	mr := multipart.NewReader(strings.NewReader(string(cl.Body)), params["boundary"])
	parts := map[string][]byte{}
	types := map[string]string{}
	for {
		p, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(p)
		parts[p.FormName()], types[p.FormName()] = b, p.Header.Get("Content-Type")
	}
	if string(parts["invoiceFile"]) != invoiceXML {
		t.Fatal("invoiceFile part")
	}
	var info map[string]any
	if err := json.Unmarshal(parts["additionalInformation"], &info); err != nil || types["additionalInformation"] != "application/json" {
		t.Fatalf("additionalInformation %s %v", parts["additionalInformation"], err)
	}
	if info["source"] != "PARTNER" || info["userId"] != float64(11) || info["companyId"] != float64(22) || info["targetAlias"] != "urn:mail:defaultpk@x.com" || info["autoInvoiceId"] != false {
		t.Fatalf("info %v", info)
	}
}

func TestStatusAndReloginOn401(t *testing.T) {
	var n int
	f, srv := newServer(t, map[string]func(http.ResponseWriter, call){
		"/api/invoice/documents/outgoing-einvoice/status/u1": func(w http.ResponseWriter, c call) {
			n++
			if n == 1 {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = io.WriteString(w, `{"title":"Unauthorized","status":401}`)
				return
			}
			jsonReply(`{"status":"105","gibStatusCode":1300,"invoiceUuid":"u1"}`)(w, c)
		},
	})
	c := newClient(t, srv.URL)
	st, err := c.Status(context.Background(), provider.StatusQuery{Kind: provider.EInvoice, UUID: "u1"})
	if err != nil {
		t.Fatal(err)
	}
	if st.Status != provider.StatusRejected || st.GIBCode != 1300 || st.ResponseCode != "RED" || f.logins != 2 {
		t.Fatalf("status %+v logins %d", st, f.logins)
	}
	for code, want := range map[string]provider.Status{"10": provider.StatusProcessing, "29": provider.StatusFailed, "40": provider.StatusSent, "205": provider.StatusDelivered, "305": provider.StatusCancelled, "405": provider.StatusFailed} {
		_, srv := newServer(t, map[string]func(http.ResponseWriter, call){
			"/api/invoice/documents/outgoing-einvoice/status/x": jsonReply(`{"status":"` + code + `","invoiceUuid":"x"}`),
		})
		st, err := newClient(t, srv.URL).Status(context.Background(), provider.StatusQuery{UUID: "x"})
		if err != nil || st.Status != want {
			t.Errorf("%s: %+v %v", code, st, err)
		}
	}
}

func TestErrors(t *testing.T) {
	for status, kind := range map[int]provider.ErrorKind{422: provider.KindValidation, 409: provider.KindInvalidState, 403: provider.KindPermission, 404: provider.KindNotFound, 500: provider.KindUnavailable} {
		_, srv := newServer(t, map[string]func(http.ResponseWriter, call){
			"/api/invoice/documents/earchive-by-file": func(w http.ResponseWriter, _ call) {
				w.Header().Set("Content-Type", "application/problem+json")
				w.WriteHeader(status)
				_, _ = io.WriteString(w, `{"title":"UBL Validation Failed","detail":"cbc:IssueDate hatalı","status":422,"code":"UBL_ERR"}`)
			},
		})
		_, err := newClient(t, srv.URL).Send(context.Background(), &provider.Document{XML: []byte(strings.Replace(invoiceXML, "TICARIFATURA", "EARSIVFATURA", 1))})
		var pe *provider.Error
		if !errors.As(err, &pe) || pe.Kind != kind || pe.Code != "UBL_ERR" || pe.Message != "cbc:IssueDate hatalı" || pe.HTTPStatus != status {
			t.Errorf("%d: %v", status, err)
		}
	}
	_, srv := newServer(t, nil)
	c, _ := trendyol.New(trendyol.Config{Email: "api@firma.com", Password: "wrong", UserID: 1, CompanyID: 2, BaseURL: srv.URL})
	if _, err := c.Status(context.Background(), provider.StatusQuery{UUID: "x"}); !errors.Is(err, provider.ErrPermission) {
		t.Fatalf("bad password: %v", err)
	}
}

func TestMarketplaceListFetchRespond(t *testing.T) {
	f, srv := newServer(t, map[string]func(http.ResponseWriter, call){
		"/api/invoice/partners/customer/signin": jsonReply(`{"userId":5,"companyId":6,"partnerCustomerId":7,"accessToken":"customer-token"}`),
		"/api/invoice/documents/incoming-einvoice/search": jsonReply(`{"pageSize":2,"currentPage":0,"totalPageCount":3,"totalCount":5,"content":[
{"invoiceUuid":"a1","invoiceId":"ABC2026000000001","status":"50","gibStatusCode":1300,"currency":"TRY","payableAmount":11455,"scenario":"TICARIFATURA","invoiceTypeCode":"SATIS","issuedAt":"2026-01-02T10:00:00Z","createdAt":"2026-01-02T10:05:00Z","senderTitle":"Satıcı A.Ş.","senderTaxId":"1111111111"},
{"invoiceUuid":"a2","status":"205","replyType":"APPROVED","payableAmount":5}]}`),
		"/api/invoice/documents/download": func(w http.ResponseWriter, _ call) {
			_, _ = io.WriteString(w, srvURL+"/dl/file")
		},
		"/api/invoice/documents/sent-reply": func(http.ResponseWriter, call) {},
	})
	srvURL = srv.URL
	c, err := trendyol.New(trendyol.Config{Email: "api@firma.com", Password: "pw", CustomerEmail: "m@x.com", CustomerPassword: "cpw", CustomerTaxID: "3333333333", BaseURL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	page, err := c.ListInbox(context.Background(), provider.ListQuery{From: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), To: time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC), Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 2 || page.Next != "1" || page.Items[0].Payable != "114.55" || page.Items[1].Payable != "0.05" || page.Items[1].Status != provider.StatusAccepted || page.Items[0].IssueDate != "2026-01-02" {
		t.Fatalf("page %+v", page)
	}
	cs := f.last("/api/invoice/partners/customer/signin")
	if cs.Header.Get("x-access-token") != "tok1" || !strings.Contains(string(cs.Body), `"taxId":"3333333333"`) {
		t.Fatalf("customer signin %v %s", cs.Header, cs.Body)
	}
	sr := f.last("/api/invoice/documents/incoming-einvoice/search")
	if sr.Header.Get("x-access-token") != "customer-token" || !strings.Contains(string(sr.Body), `"companyId":6`) || !strings.Contains(string(sr.Body), `"startDate":"2026-01-01"`) || !strings.Contains(string(sr.Body), `"size":2`) {
		t.Fatalf("search %s", sr.Body)
	}
	got, err := c.Fetch(context.Background(), provider.FetchQuery{Kind: provider.EInvoice, Direction: provider.Inbound, UUID: "a1", Format: provider.FormatPDF})
	if err != nil || string(got.Data) != "%PDF-1.4" {
		t.Fatalf("fetch %v %v", got, err)
	}
	if b := string(f.last("/api/invoice/documents/download").Body); !strings.Contains(b, `"documentType":"INCOMING_EINVOICE"`) || !strings.Contains(b, `"fileExtension":"pdf"`) {
		t.Fatalf("download body %s", b)
	}
	if err := c.Respond(context.Background(), provider.Response{UUID: "a1", Accept: false, Reason: "eksik teslimat"}); err != nil {
		t.Fatal(err)
	}
	if b := string(f.last("/api/invoice/documents/sent-reply").Body); !strings.Contains(b, `"responseType":"DENIED"`) || !strings.Contains(b, `"note":"eksik teslimat"`) || !strings.Contains(b, `"companyId":6`) {
		t.Fatalf("reply body %s", b)
	}
}

var srvURL string

func TestCheckUserAndCancel(t *testing.T) {
	f, srv := newServer(t, map[string]func(http.ResponseWriter, call){
		"/api/invoice/taxpayers/1234567890": jsonReply(`[{"taxId":"1234567890","alias":"urn:mail:defaultpk@x.com","title":"Örnek A.Ş.","gibUserType":"OZEL","sourceFile":"x","createdAt":"2024-01-01T00:00:00Z","aliasCreationTime":"2024-01-01T00:00:00Z","postBoxType":"PK","aliasType":"INVOICE"},
{"taxId":"1234567890","alias":"urn:mail:irsaliyepk@x.com","title":"Örnek A.Ş.","gibUserType":"OZEL","sourceFile":"x","createdAt":"2024-01-01T00:00:00Z","aliasCreationTime":"2024-01-01T00:00:00Z","postBoxType":"PK","aliasType":"DESPATCH_ADVICE"}]`),
		"/api/invoice/taxpayers/9999999999": func(w http.ResponseWriter, _ call) {
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"taxId":"9999999999"}`)
		},
		"/api/invoice/documents/earchive/cancel": func(http.ResponseWriter, call) {},
	})
	c := newClient(t, srv.URL)
	u, err := c.CheckUser(context.Background(), "1234567890")
	if err != nil || !u.Registered || len(u.Aliases) != 2 || u.Aliases[1].Kind != provider.EDespatch || u.Aliases[0].CreatedAt.IsZero() {
		t.Fatalf("user %+v %v", u, err)
	}
	u, err = c.CheckUser(context.Background(), "9999999999")
	if err != nil || u.Registered {
		t.Fatalf("unregistered %+v %v", u, err)
	}
	if err := c.CancelEArchive(context.Background(), "e1", "x"); err != nil {
		t.Fatal(err)
	}
	if b := string(f.last("/api/invoice/documents/earchive/cancel").Body); !strings.Contains(b, `"invoiceUuid":"e1"`) || !strings.Contains(b, `"companyId":22`) {
		t.Fatalf("cancel body %s", b)
	}
}
