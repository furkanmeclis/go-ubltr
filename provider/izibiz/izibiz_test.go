package izibiz_test

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

	"github.com/furkanmeclis/go-ubltr/provider"
	"github.com/furkanmeclis/go-ubltr/provider/izibiz"
	"github.com/furkanmeclis/go-ubltr/provider/pack"
	"github.com/furkanmeclis/go-ubltr/provider/providertest"
)

const testUUID = "8d74a41d-729e-4301-a95b-7d582004dc44"

func fixture(t testing.TB, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "fixtures", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func envelope(root, body string) string {
	return `<S:Envelope xmlns:S="http://schemas.xmlsoap.org/soap/envelope/"><S:Body><ns2:` + root +
		` xmlns:ns2="http://schemas.i2i.com/ei/wsdl">` + body + `</ns2:` + root + `></S:Body></S:Envelope>`
}

const okReturn = `<REQUEST_RETURN><INTL_TXN_ID>1</INTL_TXN_ID><RETURN_CODE>0</RETURN_CODE></REQUEST_RETURN>`

// request is one call seen by the fake server.
type request struct {
	Path, Root, Namespace, Body, Session string
}

// server replays fixtures. Handlers are keyed by request root element; a
// handler returns the response envelope for the n-th call (0-based).
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
	req := request{Path: r.URL.Path, Body: string(body)}
	req.Root, req.Namespace = bodyRoot(body)
	if i := strings.Index(req.Body, "<SESSION_ID>"); i >= 0 {
		rest := req.Body[i+len("<SESSION_ID>"):]
		req.Session = rest[:strings.Index(rest, "<")]
	}
	if r.Header.Get("SOAPAction") != `""` {
		s.t.Errorf("SOAPAction = %q, want empty quoted string", r.Header.Get("SOAPAction"))
	}
	if dir := os.Getenv("SOAP_DUMP_DIR"); dir != "" {
		// Request bodies are written for offline XSD validation.
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
		_, _ = io.WriteString(w, h(n, req))
	case req.Root == "LoginRequest":
		_, _ = io.WriteString(w, strings.ReplaceAll(fixture(s.t, "login_ok.xml"), "{{SESSION}}", fmt.Sprintf("S%d", logins)))
	default:
		s.t.Errorf("unexpected request %s at %s", req.Root, req.Path)
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

func newProvider(t testing.TB, s *server) *izibiz.Provider {
	t.Helper()
	p, err := izibiz.New(izibiz.Config{Username: "u", Password: "p", Endpoint: s.srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func invoiceXML(profile string) []byte {
	return []byte(`<?xml version="1.0" encoding="UTF-8"?>
<Invoice xmlns="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2" xmlns:cac="urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2" xmlns:cbc="urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2">
  <cbc:ProfileID>` + profile + `</cbc:ProfileID>
  <cbc:ID>ABC2026000000001</cbc:ID>
  <cbc:UUID>` + testUUID + `</cbc:UUID>
  <cbc:InvoiceTypeCode>SATIS</cbc:InvoiceTypeCode>
  <cac:AccountingCustomerParty><cac:Party><cac:PartyIdentification><cbc:ID schemeID="VKN">4840847211</cbc:ID></cac:PartyIdentification></cac:Party></cac:AccountingCustomerParty>
</Invoice>`)
}

// zipped returns base64(zip(data)) applied n times.
func zipped(t testing.TB, data []byte, n int) string {
	t.Helper()
	for i := 0; i < n; i++ {
		z, err := pack.Zip("f.xml", data)
		if err != nil {
			t.Fatal(err)
		}
		data = z
	}
	return pack.Base64(data)
}

// contentOf returns the decoded and unzipped CONTENT-like element value.
func contentOf(t testing.TB, body, elem string) []byte {
	t.Helper()
	i := strings.Index(body, "<"+elem+">")
	if i < 0 {
		t.Fatalf("no <%s> in %s", elem, body)
	}
	rest := body[i+len(elem)+2:]
	raw, err := pack.FromBase64(rest[:strings.Index(rest, "<")])
	if err != nil {
		t.Fatal(err)
	}
	_, x, err := pack.UnzipOne(raw)
	if err != nil {
		t.Fatal(err)
	}
	return x
}

func TestConformance(t *testing.T) {
	p, err := izibiz.New(izibiz.Config{Username: "u", Password: "p", Test: true})
	if err != nil {
		t.Fatal(err)
	}
	providertest.CheckCapabilities(t, p)
	providertest.CheckCodeTable(t, izibiz.Codes)
	if !provider.Supports(p, provider.EArchive, provider.OpCancelEArchive) || provider.Supports(p, provider.EDespatch, provider.OpFetch) {
		t.Error("unexpected capability set")
	}
}

func TestRegistryAndConfig(t *testing.T) {
	if _, err := provider.Open("izibiz", provider.Config{Username: "u", Password: "p"}); err == nil {
		t.Error("production without Endpoint must fail")
	}
	p, err := provider.Open("izibiz", provider.Config{Username: "u", Password: "p", Test: true, Options: map[string]string{"earchive_type": "internet"}})
	if err != nil || p.Name() != "izibiz" {
		t.Fatalf("Open: %v %v", p, err)
	}
	if _, err := izibiz.New(izibiz.Config{Username: "u", Password: "p", Test: true, EArchiveType: "X"}); err == nil {
		t.Error("bad EArchiveType accepted")
	}
}

func TestSendInvoice(t *testing.T) {
	s := newServer(t)
	s.on("SendInvoiceRequest", func(int, request) string { return fixture(t, "send_invoice_ok.xml") })
	p := newProvider(t, s)
	res, err := p.Send(context.Background(), &provider.Document{XML: invoiceXML("TICARIFATURA"), ReceiverAlias: "urn:mail:defaultpk@izibiz.com.tr"})
	if err != nil {
		t.Fatal(err)
	}
	if res.ID != "UGR2026000000320" || res.UUID != testUUID || res.Status != provider.StatusQueued || res.ProviderCode != "0" {
		t.Fatalf("result %+v", res)
	}
	r := s.last("SendInvoiceRequest")
	if r.Path != "/EInvoiceWS" || r.Namespace != "http://schemas.i2i.com/ei/wsdl" || r.Session != "S1" {
		t.Fatalf("request %+v", r)
	}
	for _, want := range []string{
		`<wsdl:SendInvoiceRequest xmlns:wsdl="http://schemas.i2i.com/ei/wsdl"><REQUEST_HEADER>`,
		`<COMPRESSED>Y</COMPRESSED>`,
		`<APPLICATION_NAME>go-ubltr</APPLICATION_NAME>`,
		`<RECEIVER vkn="4840847211" alias="urn:mail:defaultpk@izibiz.com.tr"></RECEIVER>`,
	} {
		if !strings.Contains(r.Body, want) {
			t.Errorf("request lacks %s\n%s", want, r.Body)
		}
	}
	if got := contentOf(t, r.Body, "CONTENT"); !bytes.Equal(got, invoiceXML("TICARIFATURA")) {
		t.Errorf("CONTENT = %s", got)
	}
}

func TestSessionReloginOnErrorType(t *testing.T) {
	s := newServer(t)
	s.on("SendInvoiceRequest", func(n int, _ request) string {
		if n == 0 {
			return strings.ReplaceAll(fixture(t, "invalid_session.xml"), "{{ROOT}}", "SendInvoiceResponse")
		}
		return fixture(t, "send_invoice_ok.xml")
	})
	p := newProvider(t, s)
	if _, err := p.Send(context.Background(), &provider.Document{XML: invoiceXML("TEMELFATURA")}); err != nil {
		t.Fatal(err)
	}
	if s.logins != 2 || s.last("SendInvoiceRequest").Session != "S2" {
		t.Fatalf("logins=%d session=%s", s.logins, s.last("SendInvoiceRequest").Session)
	}
}

func TestSessionReloginOnFault(t *testing.T) {
	s := newServer(t)
	s.on("GetInvoiceStatusRequest", func(n int, _ request) string {
		if n == 0 {
			return fixture(t, "request_fault.xml")
		}
		return fixture(t, "invoice_status.xml")
	})
	p := newProvider(t, s)
	if _, err := p.Status(context.Background(), provider.StatusQuery{Kind: provider.EInvoice, UUID: testUUID}); err != nil {
		t.Fatal(err)
	}
	if s.logins != 2 {
		t.Fatalf("logins = %d", s.logins)
	}
}

func TestSessionErrorTwiceIsReturned(t *testing.T) {
	s := newServer(t)
	s.on("GetInvoiceStatusRequest", func(int, request) string { return fixture(t, "request_fault.xml") })
	p := newProvider(t, s)
	_, err := p.Status(context.Background(), provider.StatusQuery{UUID: testUUID})
	var pe *provider.Error
	if !errors.As(err, &pe) || pe.Code != "2005" || pe.Kind != provider.KindAuth || pe.Op != "status" {
		t.Fatalf("err = %#v", err)
	}
	if s.logins != 2 {
		t.Fatalf("logins = %d", s.logins)
	}
}

func TestConcurrentCallsShareOneLogin(t *testing.T) {
	s := newServer(t)
	s.on("GetInvoiceStatusRequest", func(int, request) string { return fixture(t, "invoice_status.xml") })
	p := newProvider(t, s)
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := p.Status(context.Background(), provider.StatusQuery{UUID: testUUID}); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if s.logins != 1 {
		t.Fatalf("logins = %d", s.logins)
	}
}

func TestLoginBadCredentials(t *testing.T) {
	s := newServer(t)
	s.on("LoginRequest", func(int, request) string { return fixture(t, "login_bad_credentials.xml") })
	p := newProvider(t, s)
	_, err := p.Status(context.Background(), provider.StatusQuery{UUID: testUUID})
	var pe *provider.Error
	if !errors.Is(err, provider.ErrAuth) || !errors.As(err, &pe) || pe.Code != "10004" || !strings.Contains(pe.Message, "şifre") {
		t.Fatalf("err = %v", err)
	}
	if s.logins != 1 {
		t.Fatalf("login retried: %d", s.logins)
	}
	r := s.last("LoginRequest")
	for _, want := range []string{"<SESSION_ID>-1</SESSION_ID>", "<USER_NAME>u</USER_NAME>", "<PASSWORD>p</PASSWORD>"} {
		if !strings.Contains(r.Body, want) {
			t.Errorf("login lacks %s", want)
		}
	}
	if r.Path != "/AuthenticationWS" {
		t.Errorf("login path %s", r.Path)
	}
}

func TestInvoiceStatus(t *testing.T) {
	for _, tc := range []struct {
		fixture string
		want    provider.Status
		gib     int
		code    string
	}{
		{"invoice_status.xml", provider.StatusDelivered, 1300, "111"},
		{"invoice_status_sign_failed.xml", provider.StatusProcessing, -1, "106"},
	} {
		t.Run(tc.fixture, func(t *testing.T) {
			s := newServer(t)
			s.on("GetInvoiceStatusRequest", func(int, request) string { return fixture(t, tc.fixture) })
			p := newProvider(t, s)
			res, err := p.Status(context.Background(), provider.StatusQuery{Kind: provider.EInvoice, UUID: testUUID})
			if err != nil {
				t.Fatal(err)
			}
			if res.Status != tc.want || res.GIBCode != tc.gib || res.ProviderCode != tc.code || res.ID == "" || res.Message == "" {
				t.Fatalf("result %+v", res)
			}
			if !strings.Contains(s.last("GetInvoiceStatusRequest").Body, `<INVOICE UUID="`+testUUID+`"></INVOICE>`) {
				t.Error("request lacks INVOICE UUID")
			}
		})
	}
}

func TestCheckUser(t *testing.T) {
	s := newServer(t)
	s.on("CheckUserRequest", func(n int, _ request) string {
		if n == 0 {
			return fixture(t, "check_user.xml")
		}
		return fixture(t, "check_user_invalid.xml")
	})
	p := newProvider(t, s)
	u, err := p.CheckUser(context.Background(), "4840847211")
	if err != nil {
		t.Fatal(err)
	}
	if !u.Registered || u.Title != "İZİBİZ AKTİVASYON" || len(u.Aliases) != 1 || u.Aliases[0].Type != "PK" || u.Aliases[0].Kind != provider.EInvoice || u.Aliases[0].CreatedAt.IsZero() {
		t.Fatalf("user %+v", u)
	}
	r := s.last("CheckUserRequest")
	if r.Path != "/AuthenticationWS" || !strings.Contains(r.Body, "<USER><IDENTIFIER>4840847211</IDENTIFIER></USER><DOCUMENT_TYPE>INVOICE</DOCUMENT_TYPE>") {
		t.Fatalf("request %+v", r)
	}
	_, err = p.CheckUser(context.Background(), "x")
	if !errors.Is(err, provider.ErrInvalidRequest) || !strings.Contains(err.Error(), "alias,identifier") {
		t.Fatalf("err = %v", err)
	}
}

func TestListInbox(t *testing.T) {
	s := newServer(t)
	s.on("GetInvoiceRequest", func(int, request) string { return fixture(t, "get_invoice_headers.xml") })
	p := newProvider(t, s)
	page, err := p.ListInbox(context.Background(), provider.ListQuery{OnlyNew: true, Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 2 || page.Next != "" {
		t.Fatalf("page %+v", page)
	}
	a := page.Items[0]
	if a.Direction != provider.Inbound || a.Status != provider.StatusReceived || a.Profile != "TICARIFATURA" || a.Payable != "1200.00" || a.Currency != "TRY" || a.SenderVKN != "1234567801" || a.CreatedAt.IsZero() || a.GIBCode != 1300 {
		t.Fatalf("item %+v", a)
	}
	if page.Items[1].GIBCode != 0 || page.Items[1].Status != provider.StatusReceived {
		t.Fatalf("item %+v", page.Items[1])
	}
	body := s.last("GetInvoiceRequest").Body
	if !strings.Contains(body, "<INVOICE_SEARCH_KEY><LIMIT>100</LIMIT><READ_INCLUDED>false</READ_INCLUDED><DIRECTION>IN</DIRECTION></INVOICE_SEARCH_KEY><HEADER_ONLY>Y</HEADER_ONLY>") {
		t.Fatalf("request %s", body)
	}
	if _, err := p.ListOutbox(context.Background(), provider.ListQuery{Cursor: "x"}); !errors.Is(err, provider.ErrInvalidRequest) {
		t.Fatalf("cursor err = %v", err)
	}
	if _, err := p.ListInbox(context.Background(), provider.ListQuery{Kind: provider.EArchive}); !errors.Is(err, provider.ErrUnsupported) {
		t.Fatalf("kind err = %v", err)
	}
}

func TestFetchInvoice(t *testing.T) {
	s := newServer(t)
	xmlDoc := invoiceXML("TEMELFATURA")
	pdf := []byte("%PDF-1.4 test")
	s.on("GetInvoiceRequest", func(int, request) string {
		return envelope("GetInvoiceResponse", `<INVOICE ID="ABC2026000000001" UUID="`+testUUID+`"><CONTENT>`+zipped(t, xmlDoc, 1)+`</CONTENT></INVOICE>`)
	})
	s.on("GetInvoiceWithTypeRequest", func(int, request) string {
		return envelope("GetInvoiceWithTypeResponse", `<INVOICE UUID="`+testUUID+`"><CONTENT>`+zipped(t, pdf, 1)+`</CONTENT></INVOICE>`)
	})
	p := newProvider(t, s)
	res, err := p.Fetch(context.Background(), provider.FetchQuery{UUID: testUUID, Direction: provider.Inbound})
	if err != nil || !bytes.Equal(res.Data, xmlDoc) || res.Format != provider.FormatUBL {
		t.Fatalf("fetch ubl: %v %v", res, err)
	}
	if b := s.last("GetInvoiceRequest").Body; !strings.Contains(b, "<UUID>"+testUUID+"</UUID><READ_INCLUDED>true</READ_INCLUDED><DIRECTION>IN</DIRECTION>") || !strings.Contains(b, "<HEADER_ONLY>N</HEADER_ONLY>") {
		t.Fatalf("request %s", b)
	}
	res, err = p.Fetch(context.Background(), provider.FetchQuery{UUID: testUUID, Format: provider.FormatPDF})
	if err != nil || !bytes.Equal(res.Data, pdf) || res.ContentType != "application/pdf" {
		t.Fatalf("fetch pdf: %v %v", res, err)
	}
	if b := s.last("GetInvoiceWithTypeRequest").Body; !strings.Contains(b, "<TYPE>PDF</TYPE>") || !strings.Contains(b, "<DIRECTION>OUT</DIRECTION>") {
		t.Fatalf("request %s", b)
	}
	s.on("GetInvoiceRequest", func(int, request) string { return envelope("GetInvoiceResponse", "") })
	if _, err := p.Fetch(context.Background(), provider.FetchQuery{UUID: testUUID}); !errors.Is(err, provider.ErrNotFound) {
		t.Fatalf("missing: %v", err)
	}
}

func TestRespondAndMark(t *testing.T) {
	s := newServer(t)
	s.on("SendInvoiceResponseWithServerSignRequest", func(int, request) string {
		return envelope("SendInvoiceResponseWithServerSignResponse", okReturn)
	})
	s.on("MarkInvoiceRequest", func(int, request) string { return envelope("MarkInvoiceResponse", okReturn) })
	p := newProvider(t, s)
	if err := p.Respond(context.Background(), provider.Response{UUID: testUUID, Accept: false, Reason: "Hatalı tutar"}); err != nil {
		t.Fatal(err)
	}
	if b := s.last("SendInvoiceResponseWithServerSignRequest").Body; !strings.Contains(b, `<STATUS>RED</STATUS><INVOICE UUID="`+testUUID+`"></INVOICE><DESCRIPTION>Hatalı tutar</DESCRIPTION>`) {
		t.Fatalf("respond request %s", b)
	}
	if err := p.MarkTaken(context.Background(), provider.Inbound, "a", "b"); err != nil {
		t.Fatal(err)
	}
	if b := s.last("MarkInvoiceRequest").Body; !strings.Contains(b, `<MARK value="READ"><INVOICE UUID="a"></INVOICE><INVOICE UUID="b"></INVOICE></MARK>`) {
		t.Fatalf("mark request %s", b)
	}
}

func TestEArchive(t *testing.T) {
	s := newServer(t)
	pdf := []byte("%PDF-1.4 arsiv")
	html := []byte("<html>arsiv</html>")
	s.on("ArchiveInvoiceExtendedRequest", func(int, request) string {
		return archiveEnvelope("ArchiveInvoiceExtendedResponse", okReturn+`<INVOICE_ID>EAR2026000000001</INVOICE_ID>`)
	})
	s.on("GetEArchiveInvoiceStatusRequest", func(int, request) string {
		return archiveEnvelope("GetEArchiveInvoiceStatusResponse", `<INVOICE><HEADER><INVOICE_ID>EAR2026000000001</INVOICE_ID><PROFILE>EARSIVFATURA</PROFILE><UUID>`+testUUID+`</UUID><STATUS>130</STATUS><STATUS_DESC>RAPORLANDI</STATUS_DESC><EMAIL_STATUS>120</EMAIL_STATUS></HEADER></INVOICE>`+okReturn)
	})
	s.on("ArchiveInvoiceReadRequest", func(n int, _ request) string {
		if n == 0 {
			return archiveEnvelope("ArchiveInvoiceReadResponse", `<INVOICE>`+zipped(t, pdf, 1)+`</INVOICE>`+okReturn)
		}
		return archiveEnvelope("ArchiveInvoiceReadResponse", `<INVOICE>`+zipped(t, html, 2)+`</INVOICE>`+okReturn)
	})
	s.on("CancelEArchiveInvoiceRequest", func(int, request) string {
		return archiveEnvelope("CancelEArchiveInvoiceResponse", okReturn)
	})
	p := newProvider(t, s)
	ctx := context.Background()
	res, err := p.Send(ctx, &provider.Document{XML: invoiceXML("EARSIVFATURA"), Email: "alici@example.com"})
	if err != nil || res.ID != "EAR2026000000001" {
		t.Fatalf("send: %+v %v", res, err)
	}
	r := s.last("ArchiveInvoiceExtendedRequest")
	if r.Path != "/EIArchiveWS/EFaturaArchive" || r.Namespace != "http://schemas.i2i.com/ei/wsdl/archive" {
		t.Fatalf("request %+v", r)
	}
	if !strings.Contains(r.Body, "<EARSIV_FLAG>Y</EARSIV_FLAG><EARSIV_PROPERTIES><EARSIV_TYPE>NORMAL</EARSIV_TYPE><EARSIV_EMAIL_FLAG>Y</EARSIV_EMAIL_FLAG><EARSIV_EMAIL>alici@example.com</EARSIV_EMAIL></EARSIV_PROPERTIES><INVOICE_CONTENT>") {
		t.Fatalf("request %s", r.Body)
	}
	if got := contentOf(t, r.Body, "INVOICE_CONTENT"); !bytes.Equal(got, invoiceXML("EARSIVFATURA")) {
		t.Fatalf("content %s", got)
	}
	st, err := p.Status(ctx, provider.StatusQuery{Kind: provider.EArchive, UUID: testUUID})
	if err != nil || st.Status != provider.StatusDelivered || st.ProviderCode != "130" || st.ID != "EAR2026000000001" {
		t.Fatalf("status %+v %v", st, err)
	}
	f, err := p.Fetch(ctx, provider.FetchQuery{Kind: provider.EArchive, UUID: testUUID, Format: provider.FormatPDF})
	if err != nil || !bytes.Equal(f.Data, pdf) {
		t.Fatalf("fetch pdf %v %v", f, err)
	}
	if b := s.last("ArchiveInvoiceReadRequest").Body; !strings.Contains(b, "<INVOICEID>"+testUUID+"</INVOICEID><PORTAL_DIRECTION>OUT</PORTAL_DIRECTION><PROFILE>PDF</PROFILE>") {
		t.Fatalf("read request %s", b)
	}
	f, err = p.Fetch(ctx, provider.FetchQuery{Kind: provider.EArchive, UUID: testUUID, Format: provider.FormatHTML})
	if err != nil || !bytes.Equal(f.Data, html) {
		t.Fatalf("fetch html %v %v", f, err)
	}
	if err := p.CancelEArchive(ctx, testUUID, "İade"); err != nil {
		t.Fatal(err)
	}
	if b := s.last("CancelEArchiveInvoiceRequest").Body; !strings.Contains(b, "<CancelEArsivInvoiceContent><FATURA_UUID>"+testUUID+"</FATURA_UUID><IPTAL_NOTU>İade</IPTAL_NOTU></CancelEArsivInvoiceContent>") {
		t.Fatalf("cancel request %s", b)
	}
}

func TestEArchiveFault(t *testing.T) {
	s := newServer(t)
	s.on("CancelEArchiveInvoiceRequest", func(int, request) string {
		return `<S:Envelope xmlns:S="http://schemas.xmlsoap.org/soap/envelope/"><S:Body><S:Fault><faultcode>S:Server</faultcode><faultstring>ProcessingFault</faultstring><detail><ns2:RequestFault xmlns:ns2="http://schemas.i2i.com/ei/wsdl/archive"><RequestFaultResponse><INTL_TXN_ID>9</INTL_TXN_ID><ERROR_CODE>10008</ERROR_CODE><ERROR_SHORT_DES>Belirtilen kriterlere uygun kayıt bulunamamıştır</ERROR_SHORT_DES></RequestFaultResponse></ns2:RequestFault></detail></S:Fault></S:Body></S:Envelope>`
	})
	p := newProvider(t, s)
	err := p.CancelEArchive(context.Background(), testUUID, "")
	var pe *provider.Error
	if !errors.Is(err, provider.ErrNotFound) || !errors.As(err, &pe) || pe.Code != "10008" || pe.Op != "cancel-earchive" {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(s.last("CancelEArchiveInvoiceRequest").Body, "IPTAL_NOTU") {
		t.Error("empty reason sent")
	}
}

func archiveEnvelope(root, body string) string {
	return `<S:Envelope xmlns:S="http://schemas.xmlsoap.org/soap/envelope/"><S:Body><ns3:` + root +
		` xmlns:ns3="http://schemas.i2i.com/ei/wsdl/archive">` + body + `</ns3:` + root + `></S:Body></S:Envelope>`
}

func TestDespatch(t *testing.T) {
	s := newServer(t)
	s.on("SendDespatchAdviceRequest", func(int, request) string {
		return envelope("SendDespatchAdviceResponse", `<DESPATCH_ID>IRS2026000000001</DESPATCH_ID>`+okReturn)
	})
	s.on("GetDespatchAdviceStatusRequest", func(int, request) string {
		return envelope("GetDespatchAdviceStatusResponse", `<DESPATCHADVICE_STATUS><ID>IRS2026000000001</ID><UUID>`+testUUID+`</UUID><STATUS>SEND - FAILED</STATUS><STATUS_CODE>136</STATUS_CODE><STATUS_DESCRIPTION>GÖNDERME İŞLEMİ BAŞARISIZ</STATUS_DESCRIPTION><GIB_STATUS_CODE>1160</GIB_STATUS_CODE></DESPATCHADVICE_STATUS>`)
	})
	p := newProvider(t, s)
	doc := []byte(`<DespatchAdvice xmlns="urn:oasis:names:specification:ubl:schema:xsd:DespatchAdvice-2" xmlns:cbc="urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2"><cbc:ID>IRS2026000000001</cbc:ID><cbc:UUID>` + testUUID + `</cbc:UUID></DespatchAdvice>`)
	res, err := p.Send(context.Background(), &provider.Document{XML: doc, ReceiverVKN: "3900892152", ReceiverAlias: "urn:mail:irsaliyepk@gib.gov.tr"})
	if err != nil || res.ID != "IRS2026000000001" {
		t.Fatalf("send %+v %v", res, err)
	}
	r := s.last("SendDespatchAdviceRequest")
	if r.Path != "/EIrsaliyeWS/EIrsaliye" || !strings.Contains(r.Body, `<RECEIVER vkn="3900892152" alias="urn:mail:irsaliyepk@gib.gov.tr"></RECEIVER><DESPATCHADVICE><CONTENT>`) {
		t.Fatalf("request %+v", r)
	}
	if got := contentOf(t, r.Body, "CONTENT"); !bytes.Equal(got, doc) {
		t.Fatalf("content %s", got)
	}
	st, err := p.Status(context.Background(), provider.StatusQuery{Kind: provider.EDespatch, UUID: testUUID})
	if err != nil || st.Status != provider.StatusFailed || st.GIBCode != 1160 || st.ProviderCode != "136" {
		t.Fatalf("status %+v %v", st, err)
	}
}

func TestUnsupported(t *testing.T) {
	p, _ := izibiz.New(izibiz.Config{Username: "u", Password: "p", Test: true})
	doc := []byte(`<ReceiptAdvice xmlns="urn:oasis:names:specification:ubl:schema:xsd:ReceiptAdvice-2" xmlns:cbc="urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2"><cbc:UUID>` + testUUID + `</cbc:UUID></ReceiptAdvice>`)
	if _, err := p.Send(context.Background(), &provider.Document{XML: doc}); !errors.Is(err, provider.ErrUnsupported) {
		t.Fatalf("err = %v", err)
	}
	if _, err := p.Fetch(context.Background(), provider.FetchQuery{Kind: provider.EDespatch, UUID: testUUID}); !errors.Is(err, provider.ErrUnsupported) {
		t.Fatalf("err = %v", err)
	}
}

func TestHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	p, _ := izibiz.New(izibiz.Config{Username: "u", Password: "p", Endpoint: srv.URL})
	_, err := p.Status(context.Background(), provider.StatusQuery{UUID: testUUID})
	if !errors.Is(err, provider.ErrUnavailable) || !provider.IsRetryable(err) {
		t.Fatalf("err = %v", err)
	}
}
