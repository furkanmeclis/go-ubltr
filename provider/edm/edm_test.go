package edm_test

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/furkanmeclis/go-ubltr/provider"
	"github.com/furkanmeclis/go-ubltr/provider/edm"
	"github.com/furkanmeclis/go-ubltr/provider/pack"
	"github.com/furkanmeclis/go-ubltr/provider/providertest"
)

const testUUID = "8d74a41d-729e-4301-a95b-7d582004dc44"

// Responses are built in the shape the WSDL declares: root in
// http://tempuri.org/, unqualified children. EDM publishes no samples, so
// texts inside faults are placeholders.
func envelope(root, body string) string {
	return `<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"><s:Body><` + root +
		` xmlns="http://tempuri.org/">` + strings.ReplaceAll(body, "<REQUEST_RETURN>", `<REQUEST_RETURN xmlns="">`) +
		`</` + root + `></s:Body></s:Envelope>`
}

func fault(code, text string) string {
	return `<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"><s:Body><s:Fault><faultcode>s:Client</faultcode><faultstring xml:lang="tr-TR">` + text +
		`</faultstring><detail><RequestFault xmlns="http://tempuri.org/" xmlns:i="http://www.w3.org/2001/XMLSchema-instance"><INTL_TXN_ID xmlns="">0</INTL_TXN_ID><ERROR_CODE xmlns="">` + code +
		`</ERROR_CODE><ERROR_SHORT_DES xmlns="">` + text + `</ERROR_SHORT_DES></RequestFault></detail></s:Fault></s:Body></s:Envelope>`
}

const okReturn = `<REQUEST_RETURN><INTL_TXN_ID>1</INTL_TXN_ID><RETURN_CODE>0</RETURN_CODE></REQUEST_RETURN>`

type request struct {
	Action, Root, Namespace, Body, Session string
}

type server struct {
	t        testing.TB
	srv      *httptest.Server
	mu       sync.Mutex
	logins   int
	calls    map[string]int
	reqs     []request
	handlers map[string]func(n int, r request) string
}

func newServer(t testing.TB) *server {
	s := &server{t: t, calls: map[string]int{}, handlers: map[string]func(int, request) string{}}
	s.srv = httptest.NewServer(http.HandlerFunc(s.serve))
	t.Cleanup(s.srv.Close)
	return s
}

func (s *server) on(root string, h func(n int, r request) string) { s.handlers[root] = h }

func (s *server) serve(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	req := request{Action: strings.Trim(r.Header.Get("SOAPAction"), `"`), Body: string(body)}
	req.Root, req.Namespace = bodyRoot(body)
	if i := strings.Index(req.Body, "<SESSION_ID>"); i >= 0 {
		rest := req.Body[i+len("<SESSION_ID>"):]
		req.Session = rest[:strings.Index(rest, "<")]
	}
	if req.Action != req.Root {
		s.t.Errorf("SOAPAction %q for %s", req.Action, req.Root)
	}
	if req.Namespace != "http://tempuri.org/" {
		s.t.Errorf("root namespace %q", req.Namespace)
	}
	if dir := os.Getenv("SOAP_DUMP_DIR"); dir != "" {
		_ = os.WriteFile(filepath.Join(dir, fmt.Sprintf("%s-%s-%d.xml", s.t.Name(), req.Root, len(s.reqs))), body, 0o600)
	}
	s.mu.Lock()
	n := s.calls[req.Root]
	s.calls[req.Root]++
	s.reqs = append(s.reqs, req)
	if req.Root == "LoginRequest" {
		s.logins++
	}
	logins := s.logins
	h := s.handlers[req.Root]
	s.mu.Unlock()
	w.Header().Set("Content-Type", "text/xml; charset=utf-8")
	switch {
	case h != nil:
		out := h(n, req)
		if strings.Contains(out, "<s:Fault>") {
			w.WriteHeader(http.StatusInternalServerError)
		}
		_, _ = io.WriteString(w, out)
	case req.Root == "LoginRequest":
		_, _ = io.WriteString(w, envelope("LoginResponse", okReturn+fmt.Sprintf("<SESSION_ID xmlns=\"\">E%d</SESSION_ID>", logins)))
	default:
		s.t.Errorf("unexpected request %s", req.Root)
		w.WriteHeader(http.StatusInternalServerError)
	}
}

func (s *server) last(root string) request {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := len(s.reqs) - 1; i >= 0; i-- {
		if s.reqs[i].Root == root {
			return s.reqs[i]
		}
	}
	s.t.Fatalf("no %s request", root)
	return request{}
}

func bodyRoot(env []byte) (local, space string) {
	dec := xml.NewDecoder(bytes.NewReader(env))
	inBody := false
	for {
		tok, err := dec.Token()
		if err != nil {
			return "", ""
		}
		if se, ok := tok.(xml.StartElement); ok {
			if inBody {
				return se.Name.Local, se.Name.Space
			}
			inBody = se.Name.Local == "Body"
		}
	}
}

func newProvider(t testing.TB, s *server) *edm.Provider {
	t.Helper()
	p, err := edm.New(edm.Config{Username: "u", Password: "p", Endpoint: s.srv.URL, VKN: "1234567801"})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func invoiceXML() []byte {
	return []byte(`<?xml version="1.0" encoding="UTF-8"?>
<Invoice xmlns="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2" xmlns:cac="urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2" xmlns:cbc="urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2">
  <cbc:ProfileID>TEMELFATURA</cbc:ProfileID>
  <cbc:ID>EDM2026000000001</cbc:ID>
  <cbc:UUID>` + testUUID + `</cbc:UUID>
  <cac:AccountingCustomerParty><cac:Party><cac:PartyIdentification><cbc:ID schemeID="VKN">9876543210</cbc:ID></cac:PartyIdentification></cac:Party></cac:AccountingCustomerParty>
</Invoice>`)
}

func TestConformance(t *testing.T) {
	p, err := edm.New(edm.Config{Username: "u", Password: "p", Test: true})
	if err != nil {
		t.Fatal(err)
	}
	providertest.CheckCapabilities(t, p)
	providertest.CheckCodeTable(t, edm.Codes)
	if provider.Supports(p, provider.EInvoice, provider.OpRespond) || len(p.Capabilities().Kinds()) != 1 {
		t.Error("unexpected capability set")
	}
	if _, err := provider.Open("edm", provider.Config{Username: "u", Password: "p"}); err == nil {
		t.Error("production without Endpoint must fail")
	}
	if q, err := provider.Open("edm", provider.Config{Username: "u", Password: "p", Test: true}); err != nil || q.Name() != "edm" {
		t.Fatalf("Open: %v", err)
	}
}

func TestSend(t *testing.T) {
	s := newServer(t)
	s.on("SendInvoiceRequest", func(int, request) string {
		return envelope("SendInvoiceResponse", okReturn+`<INVOICE xmlns="" TRXID="5" UUID="`+testUUID+`" ID="EDM2026000000009"><HEADER><STATUS>PACKAGE - PROCESSING</STATUS><INTERNETSALES>false</INTERNETSALES><EARCHIVE>false</EARCHIVE></HEADER></INVOICE>`)
	})
	p := newProvider(t, s)
	res, err := p.Send(context.Background(), &provider.Document{XML: invoiceXML(), ReceiverAlias: "urn:mail:defaultpk@edmbilisim.com.tr"})
	if err != nil {
		t.Fatal(err)
	}
	if res.ID != "EDM2026000000009" || res.Status != provider.StatusProcessing || res.ProviderCode != "PACKAGE-PROCESSING" {
		t.Fatalf("result %+v", res)
	}
	r := s.last("SendInvoiceRequest")
	for _, want := range []string{
		`<tem:SendInvoiceRequest xmlns:tem="http://tempuri.org/"><REQUEST_HEADER><SESSION_ID>E1</SESSION_ID>`,
		`<REASON>go-ubltr</REASON><APPLICATION_NAME>go-ubltr</APPLICATION_NAME>`,
		`<CHANNEL_NAME>go-ubltr</CHANNEL_NAME><COMPRESSED>N</COMPRESSED></REQUEST_HEADER>`,
		`<SENDER vkn="1234567801"></SENDER><RECEIVER vkn="9876543210" alias="urn:mail:defaultpk@edmbilisim.com.tr"></RECEIVER><INVOICE TRXID="0"><CONTENT>`,
	} {
		if !strings.Contains(r.Body, want) {
			t.Errorf("request lacks %s\n%s", want, r.Body)
		}
	}
	i := strings.Index(r.Body, "<CONTENT>") + len("<CONTENT>")
	raw, err := pack.FromBase64(r.Body[i : i+strings.Index(r.Body[i:], "<")])
	if err != nil || !bytes.Equal(raw, invoiceXML()) {
		t.Fatalf("CONTENT is not the raw XML: %v", err)
	}
	login := s.last("LoginRequest")
	if !strings.Contains(login.Body, "<USER_NAME>u</USER_NAME><PASSWORD>p</PASSWORD>") || login.Session != "" {
		t.Fatalf("login %s", login.Body)
	}
}

func TestStatus(t *testing.T) {
	s := newServer(t)
	s.on("GetInvoiceStatusRequest", func(n int, _ request) string {
		if n == 0 {
			return envelope("GetInvoiceStatusResponse", okReturn+`<INVOICE_STATUS xmlns="" UUID="`+testUUID+`" ID="EDM2026000000001"><STATUS>SEND - SUCCEED</STATUS><STATUS_DESCRIPTION>SUCCEED</STATUS_DESCRIPTION><GIB_STATUS_CODE>1300</GIB_STATUS_CODE><DIRECTION>OUT</DIRECTION></INVOICE_STATUS>`)
		}
		return envelope("GetInvoiceStatusResponse", okReturn+`<INVOICE_STATUS xmlns="" UUID="`+testUUID+`"><STATUS>SEND - WAIT_SYSTEM_RESPONSE</STATUS><GIB_STATUS_CODE>1230</GIB_STATUS_CODE></INVOICE_STATUS>`)
	})
	p := newProvider(t, s)
	st, err := p.Status(context.Background(), provider.StatusQuery{UUID: testUUID})
	if err != nil {
		t.Fatal(err)
	}
	if st.Status != provider.StatusDelivered || st.GIBCode != 1300 || st.ProviderCode != "SEND-SUCCEED" || st.ID != "EDM2026000000001" {
		t.Fatalf("status %+v", st)
	}
	if !strings.Contains(s.last("GetInvoiceStatusRequest").Body, `<INVOICE TRXID="0" UUID="`+testUUID+`"></INVOICE>`) {
		t.Error("request lacks INVOICE UUID")
	}
	st, err = p.Status(context.Background(), provider.StatusQuery{UUID: testUUID})
	if err != nil || st.Status != provider.StatusFailed {
		t.Fatalf("GİB 1230 should fail: %+v %v", st, err)
	}
	if _, err := p.Status(context.Background(), provider.StatusQuery{Kind: provider.EArchive, UUID: testUUID}); !errors.Is(err, provider.ErrUnsupported) {
		t.Fatalf("err = %v", err)
	}
}

func TestSessionRelogin(t *testing.T) {
	s := newServer(t)
	s.on("MarkInvoiceRequest", func(n int, _ request) string {
		if n == 0 {
			return fault("-1", "Oturum bulunamadı")
		}
		return envelope("MarkInvoiceResponse", okReturn)
	})
	p := newProvider(t, s)
	if err := p.MarkTaken(context.Background(), provider.Inbound, "a", "b"); err != nil {
		t.Fatal(err)
	}
	r := s.last("MarkInvoiceRequest")
	if s.logins != 2 || r.Session != "E2" || !strings.Contains(r.Body, `<MARK value="READ"><INVOICE TRXID="0" UUID="a"></INVOICE><INVOICE TRXID="0" UUID="b"></INVOICE></MARK>`) {
		t.Fatalf("logins=%d request=%s", s.logins, r.Body)
	}
}

func TestFaultIsReturned(t *testing.T) {
	s := newServer(t)
	s.on("GetInvoiceStatusRequest", func(int, request) string { return fault("1163", "Fatura daha önce gönderilmiş") })
	p := newProvider(t, s)
	_, err := p.Status(context.Background(), provider.StatusQuery{UUID: testUUID})
	var pe *provider.Error
	if !errors.As(err, &pe) || pe.Code != "1163" || pe.GIBCode != 1163 || !errors.Is(err, provider.ErrDuplicate) || pe.HTTPStatus != 500 {
		t.Fatalf("err = %#v", err)
	}
	if s.logins != 1 {
		t.Fatalf("logins = %d", s.logins)
	}
}

func TestLoginFault(t *testing.T) {
	s := newServer(t)
	s.on("LoginRequest", func(int, request) string { return fault("100", "Hatalı giriş") })
	p := newProvider(t, s)
	_, err := p.CheckUser(context.Background(), "9876543210")
	if !errors.Is(err, provider.ErrAuth) {
		t.Fatalf("err = %v", err)
	}
}

func TestConcurrentCallsShareOneLogin(t *testing.T) {
	s := newServer(t)
	s.on("CheckUserRequest", func(int, request) string { return envelope("CheckUserResponse", "") })
	p := newProvider(t, s)
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := p.CheckUser(context.Background(), "1"); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if s.logins != 1 {
		t.Fatalf("logins = %d", s.logins)
	}
}

func TestListAndFetch(t *testing.T) {
	headers, err := os.ReadFile(filepath.Join("testdata", "fixtures", "get_invoice_headers.xml"))
	if err != nil {
		t.Fatal(err)
	}
	pdf := []byte("%PDF-1.4 edm")
	s := newServer(t)
	s.on("GetInvoiceRequest", func(n int, _ request) string {
		switch n {
		case 0:
			return string(headers)
		case 1:
			return envelope("GetInvoiceResponse", `<INVOICE xmlns="" UUID="`+testUUID+`"><CONTENT>`+pack.Base64(invoiceXML())+`</CONTENT></INVOICE>`)
		}
		z, _ := pack.Zip("a.pdf", pdf)
		return envelope("GetInvoiceResponse", `<INVOICE xmlns="" UUID="`+testUUID+`"><CONTENT>`+pack.Base64(z)+`</CONTENT></INVOICE>`)
	})
	p := newProvider(t, s)
	ctx := context.Background()
	page, err := p.ListInbox(ctx, provider.ListQuery{OnlyNew: true, Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 {
		t.Fatalf("page %+v", page)
	}
	it := page.Items[0]
	if it.Status != provider.StatusReceived || it.Direction != provider.Inbound || it.Payable != "1200.00" || it.ID != "EDM2026000000001" || it.CreatedAt.IsZero() || it.GIBCode != 1300 {
		t.Fatalf("item %+v", it)
	}
	if b := s.last("GetInvoiceRequest").Body; !strings.Contains(b, "<INVOICE_SEARCH_KEY><LIMIT>50</LIMIT><READ_INCLUDED>false</READ_INCLUDED><DIRECTION>IN</DIRECTION></INVOICE_SEARCH_KEY><HEADER_ONLY>Y</HEADER_ONLY><INVOICE_CONTENT_TYPE>XML</INVOICE_CONTENT_TYPE>") {
		t.Fatalf("list request %s", b)
	}
	f, err := p.Fetch(ctx, provider.FetchQuery{UUID: testUUID, Direction: provider.Outbound})
	if err != nil || !bytes.Equal(f.Data, invoiceXML()) {
		t.Fatalf("fetch ubl %v %v", f, err)
	}
	f, err = p.Fetch(ctx, provider.FetchQuery{UUID: testUUID, Format: provider.FormatPDF})
	if err != nil || !bytes.Equal(f.Data, pdf) || f.ContentType != "application/pdf" {
		t.Fatalf("fetch pdf %v %v", f, err)
	}
	if b := s.last("GetInvoiceRequest").Body; !strings.Contains(b, "<LIMIT>1</LIMIT><UUID>"+testUUID+"</UUID><READ_INCLUDED>true</READ_INCLUDED><DIRECTION>OUT</DIRECTION></INVOICE_SEARCH_KEY><HEADER_ONLY>N</HEADER_ONLY><INVOICE_CONTENT_TYPE>PDF</INVOICE_CONTENT_TYPE>") {
		t.Fatalf("fetch request %s", b)
	}
	if _, err := p.ListOutbox(ctx, provider.ListQuery{Cursor: "x"}); !errors.Is(err, provider.ErrInvalidRequest) {
		t.Fatalf("cursor err = %v", err)
	}
}

func TestCheckUser(t *testing.T) {
	s := newServer(t)
	s.on("CheckUserRequest", func(int, request) string {
		return envelope("CheckUserResponse", `<USER xmlns=""><IDENTIFIER>9876543210</IDENTIFIER><ALIAS>urn:mail:defaultpk@ornek.com.tr</ALIAS><TITLE>ÖRNEK ALICI LTD.</TITLE><TYPE>OZEL</TYPE><UNIT>PK</UNIT><ALIAS_CREATION_TIME>2020-01-02T10:00:00</ALIAS_CREATION_TIME></USER>`+
			`<USER xmlns=""><IDENTIFIER>9876543210</IDENTIFIER><ALIAS>urn:mail:eski@ornek.com.tr</ALIAS><UNIT>PK</UNIT><ALIAS_REMOVAL_TIME>2021-01-01T00:00:00</ALIAS_REMOVAL_TIME></USER>`)
	})
	p := newProvider(t, s)
	u, err := p.CheckUser(context.Background(), "9876543210")
	if err != nil {
		t.Fatal(err)
	}
	if !u.Registered || len(u.Aliases) != 1 || u.Title != "ÖRNEK ALICI LTD." || u.Aliases[0].Type != "PK" || u.Aliases[0].CreatedAt.IsZero() {
		t.Fatalf("user %+v", u)
	}
	if !strings.Contains(s.last("CheckUserRequest").Body, "<USER><IDENTIFIER>9876543210</IDENTIFIER></USER>") {
		t.Fatal("request lacks USER")
	}
}

func TestUnsupported(t *testing.T) {
	// The endpoint is unreachable on purpose: every case must fail before
	// any network call.
	p, _ := edm.New(edm.Config{Username: "u", Password: "p", Endpoint: "http://127.0.0.1:1/EFaturaEDM.svc"})
	ctx := context.Background()
	arsiv := bytes.Replace(invoiceXML(), []byte("TEMELFATURA"), []byte("EARSIVFATURA"), 1)
	if _, err := p.Send(ctx, &provider.Document{XML: arsiv}); !errors.Is(err, provider.ErrUnsupported) {
		t.Fatalf("e-Arşiv send: %v", err)
	}
	now := time.Now()
	if _, err := p.ListInbox(ctx, provider.ListQuery{From: now, To: now.Add(-time.Hour)}); !errors.Is(err, provider.ErrInvalidRequest) {
		t.Fatalf("reversed date filter: %v", err)
	}
	if _, err := p.ListInbox(ctx, provider.ListQuery{Cursor: "x"}); !errors.Is(err, provider.ErrInvalidRequest) {
		t.Fatalf("bad cursor: %v", err)
	}
	if _, err := p.Fetch(ctx, provider.FetchQuery{Kind: provider.EDespatch, UUID: testUUID}); !errors.Is(err, provider.ErrUnsupported) {
		t.Fatalf("despatch fetch: %v", err)
	}
}

func TestListDateWindowAndOffset(t *testing.T) {
	headers, err := os.ReadFile(filepath.Join("testdata", "fixtures", "get_invoice_headers.xml"))
	if err != nil {
		t.Fatal(err)
	}
	s := newServer(t)
	s.on("GetInvoiceRequest", func(int, request) string { return string(headers) })
	p := newProvider(t, s)
	ctx := context.Background()
	trt := time.FixedZone("TRT", 3*60*60)
	from := time.Date(2026, 11, 1, 0, 0, 0, 0, trt)
	to := time.Date(2026, 11, 2, 12, 30, 0, 0, trt)
	page, err := p.ListOutbox(ctx, provider.ListQuery{From: from, To: to, Limit: 1, Cursor: "20"})
	if err != nil {
		t.Fatal(err)
	}
	if page.Next != "21" {
		t.Fatalf("next = %q", page.Next)
	}
	b := s.last("GetInvoiceRequest").Body
	want := "<LIMIT>1</LIMIT><READ_INCLUDED>true</READ_INCLUDED><DIRECTION>OUT</DIRECTION>" +
		"<CR_START_DATE>2026-11-01T00:00:00</CR_START_DATE><CR_END_DATE>2026-11-02T12:30:00</CR_END_DATE><OFFSET>20</OFFSET></INVOICE_SEARCH_KEY>"
	if !strings.Contains(b, want) {
		t.Fatalf("request lacks %s\n%s", want, b)
	}
	// Read documents included and no dates: EDM requires a window.
	if _, err := p.ListOutbox(ctx, provider.ListQuery{}); err != nil {
		t.Fatal(err)
	}
	if b := s.last("GetInvoiceRequest").Body; !strings.Contains(b, "<CR_START_DATE>") || !strings.Contains(b, "<CR_END_DATE>") {
		t.Fatalf("default window missing\n%s", b)
	}
	// Only new documents: no window needed.
	if _, err := p.ListInbox(ctx, provider.ListQuery{OnlyNew: true}); err != nil {
		t.Fatal(err)
	}
	if b := s.last("GetInvoiceRequest").Body; strings.Contains(b, "CR_START_DATE") {
		t.Fatalf("unexpected window\n%s", b)
	}
}
