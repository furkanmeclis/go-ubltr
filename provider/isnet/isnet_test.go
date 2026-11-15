package isnet_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/furkanmeclis/go-ubltr/provider"
	"github.com/furkanmeclis/go-ubltr/provider/isnet"
	"github.com/furkanmeclis/go-ubltr/provider/providertest"
)

const (
	nsSoap  = "http://schemas.xmlsoap.org/soap/envelope/"
	nsTem   = "http://tempuri.org/"
	nsModel = "http://schemas.datacontract.org/2004/07/EInvoice.Service.Model"
	vkn     = "1234567890"
)

func invoiceXML(profile, uuid, id string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<Invoice xmlns="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2"
 xmlns:cac="urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2"
 xmlns:cbc="urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2">
 <cbc:ProfileID>` + profile + `</cbc:ProfileID>
 <cbc:ID>` + id + `</cbc:ID>
 <cbc:UUID>` + uuid + `</cbc:UUID>
 <cbc:InvoiceTypeCode>SATIS</cbc:InvoiceTypeCode>
 <cac:AccountingCustomerParty><cac:Party><cac:PartyIdentification><cbc:ID schemeID="TCKN">11111111111</cbc:ID></cac:PartyIdentification></cac:Party></cac:AccountingCustomerParty>
</Invoice>`
}

var (
	efaturaUBL = invoiceXML("TICARIFATURA", "F3B1C2D4-0000-4A5B-9C8D-123456789ABC", "ABC2026000000123")
	earsivUBL  = invoiceXML("EARSIVFATURA", "9A8B7C6D-3333-4A5B-9C8D-000000000001", "EAR2026000000001")
)

func fixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "fixtures", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func withUBL(s, ubl string) string {
	return strings.ReplaceAll(s, "{{UBL}}", base64.StdEncoding.EncodeToString([]byte(ubl)))
}

// envelope wraps a body in a WCF-style response envelope.
func envelope(method, result string) string {
	return `<s:Envelope xmlns:s="` + nsSoap + `"><s:Body><` + method + `Response xmlns="` + nsTem + `"><` + method +
		`Result xmlns:a="` + nsModel + `" xmlns:i="http://www.w3.org/2001/XMLSchema-instance">` + result +
		`</` + method + `Result></` + method + `Response></s:Body></s:Envelope>`
}

// leaf is one element of a canonical request: its namespaced path and
// text.
type leaf struct{ path, text string }

// canon reads the SOAP body of a request into document-ordered elements,
// ignoring prefixes, whitespace and comments.
func canon(t *testing.T, body []byte) (method string, out []leaf) {
	t.Helper()
	dec := xml.NewDecoder(bytes.NewReader(body))
	var stack []string
	var text strings.Builder
	inBody := false
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			return method, out
		}
		if err != nil {
			t.Fatalf("bad xml: %v\n%s", err, body)
		}
		switch v := tok.(type) {
		case xml.StartElement:
			if v.Name.Space == nsSoap && v.Name.Local == "Body" {
				inBody = true
				continue
			}
			if !inBody {
				continue
			}
			if method == "" {
				method = v.Name.Local
				if v.Name.Space != nsTem {
					t.Errorf("%s: namespace %q, want %q", method, v.Name.Space, nsTem)
				}
			}
			stack = append(stack, "{"+v.Name.Space+"}"+v.Name.Local)
			out = append(out, leaf{path: strings.Join(stack, "/")})
			text.Reset()
		case xml.CharData:
			text.Write(v)
		case xml.EndElement:
			if v.Name.Space == nsSoap {
				inBody = false
				continue
			}
			if !inBody || len(stack) == 0 {
				continue
			}
			p := strings.Join(stack, "/")
			for i := len(out) - 1; i >= 0; i-- {
				if out[i].path == p {
					out[i].text = strings.TrimSpace(text.String())
					break
				}
			}
			stack = stack[:len(stack)-1]
			text.Reset()
		}
	}
}

// call is one recorded request.
type call struct {
	method, action, contentType string
	body                        []byte
	leaves                      []leaf
}

func (c call) get(local string) string {
	for _, l := range c.leaves {
		if strings.HasSuffix(l.path, "}"+local) {
			return l.text
		}
	}
	return ""
}

func (c call) has(local string) bool {
	for _, l := range c.leaves {
		if strings.HasSuffix(l.path, "}"+local) {
			return true
		}
	}
	return false
}

type fakeServer struct {
	t     *testing.T
	mu    sync.Mutex
	calls []call
	// reply returns the response envelope for the n-th call of a method.
	reply func(method string, n int, c call) string
}

func newServer(t *testing.T, reply func(method string, n int, c call) string) (*fakeServer, *httptest.Server) {
	f := &fakeServer{t: t, reply: reply}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		m, leaves := canon(t, body)
		c := call{method: m, action: r.Header.Get("SOAPAction"), contentType: r.Header.Get("Content-Type"), body: body, leaves: leaves}
		f.mu.Lock()
		n := 0
		for _, p := range f.calls {
			if p.method == m {
				n++
			}
		}
		f.calls = append(f.calls, c)
		f.mu.Unlock()
		checkOrder(t, m, leaves)
		out := reply(m, n, c)
		if out == "" {
			t.Errorf("unexpected call %s", m)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/xml; charset=utf-8")
		if strings.Contains(out, ":Fault>") {
			w.WriteHeader(http.StatusInternalServerError)
		}
		_, _ = io.WriteString(w, out)
	}))
	t.Cleanup(srv.Close)
	return f, srv
}

func (f *fakeServer) all(method string) []call {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []call
	for _, c := range f.calls {
		if c.method == method {
			out = append(out, c)
		}
	}
	return out
}

func (f *fakeServer) last(method string) call {
	cs := f.all(method)
	if len(cs) == 0 {
		f.t.Fatalf("no %s call", method)
	}
	return cs[len(cs)-1]
}

// checkOrder asserts that siblings under the request element are in
// ordinal alphabetical order, which WCF requires.
func checkOrder(t *testing.T, method string, leaves []leaf) {
	t.Helper()
	children := map[string][]string{}
	var parents []string
	for _, l := range leaves {
		i := strings.LastIndex(l.path, "/")
		if i < 0 {
			continue
		}
		parent := l.path[:i]
		if _, ok := children[parent]; !ok {
			parents = append(parents, parent)
		}
		name := l.path[strings.LastIndex(l.path, "}")+1:]
		children[parent] = append(children[parent], name)
	}
	for _, p := range parents {
		if strings.HasSuffix(p, "}"+method) {
			continue
		}
		if !slices.IsSorted(children[p]) {
			t.Errorf("%s: children of %s not alphabetical: %v", method, p, children[p])
		}
	}
}

// sameShape compares a recorded request with a fixture request.
func sameShape(t *testing.T, got call, fixtureName string) {
	t.Helper()
	m, want := canon(t, []byte(fixture(t, fixtureName)))
	if got.method != m {
		t.Fatalf("method %s, fixture %s", got.method, m)
	}
	if !slices.Equal(got.leaves, want) {
		t.Errorf("%s request differs from %s\n got: %v\nwant: %v\nbody: %s", m, fixtureName, got.leaves, want, got.body)
	}
	if got.action != `"http://tempuri.org/IInvoiceService/`+m+`"` && got.action != `"http://tempuri.org/IAddressBookService/`+m+`"` {
		t.Errorf("SOAPAction %s", got.action)
	}
	if got.contentType != "text/xml; charset=utf-8" {
		t.Errorf("Content-Type %s", got.contentType)
	}
}

func newClient(t *testing.T, url string, mod ...func(*isnet.Config)) *isnet.Client {
	t.Helper()
	cfg := isnet.Config{VKN: vkn, Endpoint: url, AddressBookEndpoint: url}
	for _, m := range mod {
		m(&cfg)
	}
	c, err := isnet.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestCapabilitiesAndCodes(t *testing.T) {
	c := newClient(t, "http://127.0.0.1:1")
	providertest.CheckCapabilities(t, c)
	providertest.CheckCodeTable(t, isnet.Codes)
	for _, op := range []provider.Operation{provider.OpSend, provider.OpRespond, provider.OpMarkTaken, provider.OpCancelEArchive, provider.OpCredits} {
		if provider.Supports(c, provider.EInvoice, op) || provider.Supports(c, provider.EArchive, op) {
			t.Errorf("%s must not be declared", op)
		}
	}
	if provider.Supports(c, provider.EArchive, provider.OpListInbox) || !provider.Supports(c, provider.EArchive, provider.OpListOutbox) {
		t.Error("e-Arşiv capabilities")
	}
	p, err := provider.Open("isnet", provider.Config{VKN: vkn, Test: true, Options: map[string]string{
		"vendor_numbers": "B01, B02", "timeout": "30s", "page_size": "50", "ip_filtered": "false",
	}})
	if err != nil || p.Name() != "isnet" {
		t.Fatalf("Open: %v", err)
	}
	if _, err := provider.Open("isnet", provider.Config{VKN: vkn, Options: map[string]string{"timeout": "soon"}}); err == nil {
		t.Error("bad timeout option must fail")
	}
	if _, err := isnet.New(isnet.Config{}); err == nil {
		t.Error("New without VKN must fail")
	}
	if info, ok := isnet.Codes.Lookup(isnet.CodeIPNotAllowed); !ok || info.Kind != provider.KindPermission {
		t.Errorf("ip-not-allowlisted row: %+v", info)
	}
}

func TestListInbox(t *testing.T) {
	f, srv := newServer(t, func(m string, _ int, _ call) string {
		if m == "SearchAllInvoice" {
			return withUBL(fixture(t, "search_all_invoice_response.xml"), efaturaUBL)
		}
		return ""
	})
	c := newClient(t, srv.URL, func(c *isnet.Config) { c.VendorNumbers = []string{"B01"} })
	from := time.Date(2026, 11, 1, 0, 0, 0, 0, time.FixedZone("TRT", 3*3600))
	to := time.Date(2026, 11, 30, 20, 59, 59, 0, time.UTC) // 23:59:59 in Türkiye
	page, err := c.ListInbox(context.Background(), provider.ListQuery{From: from, To: to, Cursor: "2", Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	sameShape(t, f.last("SearchAllInvoice"), "search_all_invoice_request.xml")
	if len(page.Items) != 2 || page.Next != "3" {
		t.Fatalf("items %d next %q", len(page.Items), page.Next)
	}
	d := page.Items[0]
	want := provider.DocumentInfo{
		Kind: provider.EInvoice, Direction: provider.Inbound,
		UUID: "F3B1C2D4-0000-4A5B-9C8D-123456789ABC", ID: "ABC2026000000123",
		Profile: "TICARIFATURA", Type: "SATIS", IssueDate: "2026-11-02",
		SenderVKN: "9876543210", SenderName: "Örnek Satıcı A.Ş.", ReceiverVKN: vkn,
		Currency: "TRY", Payable: "1200.00", Status: provider.StatusReceived, GIBCode: 1300,
		ProviderCode: "Onay_Bekliyor/Basariyla_Tamamlandi",
	}
	created := d.CreatedAt
	d.CreatedAt = time.Time{}
	if d != want {
		t.Fatalf("item\n got %+v\nwant %+v", d, want)
	}
	if created.UTC() != time.Date(2026, 11, 3, 7, 15, 30, 123e6, time.UTC) {
		t.Errorf("CreatedAt %v", created)
	}
	if page.Items[1].UUID != "0A1B2C3D-1111-4A5B-9C8D-000000000002" || page.Items[1].Status != provider.StatusAccepted {
		t.Errorf("Ettn spelling or status: %+v", page.Items[1])
	}
}

func TestListOutboxEInvoice(t *testing.T) {
	f, srv := newServer(t, func(string, int, call) string {
		return withUBL(fixture(t, "search_all_invoice_response.xml"), efaturaUBL)
	})
	c := newClient(t, srv.URL)
	page, err := c.ListOutbox(context.Background(), provider.ListQuery{})
	if err != nil {
		t.Fatal(err)
	}
	r := f.last("SearchAllInvoice")
	if r.get("InvoiceDirection") != "Outgoing" || r.get("RecordsPerPage") != "20" || r.get("PageNumber") != "1" ||
		r.has("MinInvoiceCreationDate") || r.has("IncludeCompanyVendorNumberList") || r.get("IsXMLIncluded") != "false" {
		t.Fatalf("request %v", r.leaves)
	}
	d := page.Items[0]
	if d.Status != provider.StatusDelivered || d.ReceiverVKN != "9876543210" || d.ReceiverName != "Örnek Satıcı A.Ş." || d.SenderVKN != vkn {
		t.Fatalf("item %+v", d)
	}
	if page.Next != "2" {
		t.Errorf("next %q", page.Next)
	}
	if _, err := c.ListInbox(context.Background(), provider.ListQuery{OnlyNew: true}); provider.KindOf(err) != provider.KindInvalidRequest {
		t.Errorf("OnlyNew: %v", err)
	}
	if _, err := c.ListInbox(context.Background(), provider.ListQuery{Kind: provider.EArchive}); !errors.Is(err, provider.ErrUnsupported) {
		t.Errorf("e-Arşiv inbox: %v", err)
	}
	if _, err := c.ListOutbox(context.Background(), provider.ListQuery{Cursor: "x"}); provider.KindOf(err) != provider.KindInvalidRequest {
		t.Errorf("bad cursor: %v", err)
	}
}

func TestListOutboxEArchive(t *testing.T) {
	f, srv := newServer(t, func(string, int, call) string {
		return withUBL(fixture(t, "search_archive_invoice_response.xml"), earsivUBL)
	})
	c := newClient(t, srv.URL)
	page, err := c.ListOutbox(context.Background(), provider.ListQuery{Kind: provider.EArchive, Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	r := f.last("SearchArchiveInvoice")
	if r.has("IsCancelled") || r.get("IsXMLIncluded") != "true" || r.get("CompanyTaxCode") != vkn || r.has("IncludeMainCompany") {
		t.Fatalf("request %v", r.leaves)
	}
	if len(page.Items) != 2 || page.Next != "2" {
		t.Fatalf("items %d next %q", len(page.Items), page.Next)
	}
	d := page.Items[0]
	if d.Kind != provider.EArchive || d.Direction != provider.Outbound || d.UUID != "9A8B7C6D-3333-4A5B-9C8D-000000000001" ||
		d.ID != "EAR2026000000001" || d.Profile != "EARSIVFATURA" || d.Status != provider.StatusDelivered ||
		d.ReceiverVKN != "11111111111" || d.IssueDate != "2026-11-05" || d.Payable != "240.00" {
		t.Fatalf("item %+v", d)
	}
	if d.ProviderCode != "Fatura_Olusturuldu/Iptal_Edilmedi/Gib_Tarafinda_Hata_Olustu" {
		t.Errorf("provider code %q", d.ProviderCode)
	}
	if e := page.Items[1]; e.Status != provider.StatusCancelled || e.UUID != "5D6E7F80-2222-4A5B-9C8D-000000000003" {
		t.Fatalf("cancelled item %+v", e)
	}

	from := time.Date(2026, 11, 1, 0, 0, 0, 0, time.FixedZone("TRT", 3*3600))
	to := time.Date(2026, 11, 30, 23, 59, 59, 0, time.FixedZone("TRT", 3*3600))
	page, err = c.ListEArchiveCancelled(context.Background(), provider.ListQuery{From: from, To: to})
	if err != nil {
		t.Fatal(err)
	}
	sameShape(t, f.last("SearchArchiveInvoice"), "search_archive_invoice_request.xml")
	for _, d := range page.Items {
		if d.Status != provider.StatusCancelled {
			t.Errorf("cancelled listing item %+v", d)
		}
	}
}

func TestFetch(t *testing.T) {
	pdf := []byte("%PDF-1.4 test")
	html := []byte("<html><body>fatura</body></html>")
	f, srv := newServer(t, func(m string, _ int, c call) string {
		switch m {
		case "SearchAllInvoice":
			if c.get("InvoiceDirection") == "Outgoing" && c.get("IsXMLIncluded") == "true" {
				return envelope(m, `<a:Invoices/><a:Result>Success</a:Result>`)
			}
			body := ""
			switch {
			case c.get("IsXMLIncluded") == "true":
				body = `<a:InvoiceXml>` + base64.StdEncoding.EncodeToString([]byte(efaturaUBL)) + `</a:InvoiceXml>`
			case c.get("IsPDFIncluded") == "true":
				body = `<a:InvoicePdf>` + base64.StdEncoding.EncodeToString(pdf) + `</a:InvoicePdf>`
			case c.get("IsHtmlIncluded") == "true":
				body = `<a:InvoiceHtml>` + base64.StdEncoding.EncodeToString(html) + `</a:InvoiceHtml>`
			}
			return envelope(m, `<a:Invoices><a:Invoice><a:ETTN>`+c.get("Ettn")+`</a:ETTN>`+body+`</a:Invoice></a:Invoices><a:Result>Success</a:Result>`)
		case "SearchArchiveInvoice":
			return envelope(m, `<a:ArchiveInvoices><a:ArchiveInvoice><a:InvoicePdf>`+base64.StdEncoding.EncodeToString(pdf)+`</a:InvoicePdf></a:ArchiveInvoice></a:ArchiveInvoices><a:Result>Success</a:Result>`)
		}
		return ""
	})
	c := newClient(t, srv.URL)
	ctx := context.Background()
	id := "F3B1C2D4-0000-4A5B-9C8D-123456789ABC"
	res, err := c.Fetch(ctx, provider.FetchQuery{UUID: id})
	if err != nil || string(res.Data) != efaturaUBL || res.ContentType != "application/xml" || res.Format != provider.FormatUBL {
		t.Fatalf("UBL: %v %v", res, err)
	}
	calls := f.all("SearchAllInvoice")
	if len(calls) != 2 || calls[0].get("InvoiceDirection") != "Outgoing" || calls[1].get("InvoiceDirection") != "Incoming" {
		t.Fatalf("direction fallback: %d calls", len(calls))
	}
	r := calls[1]
	if r.get("Ettn") != id || r.get("RecordsPerPage") != "1" || r.get("IsPDFIncluded") != "false" || r.get("IncludeMainCompany") != "true" {
		t.Fatalf("request %v", r.leaves)
	}
	res, err = c.Fetch(ctx, provider.FetchQuery{UUID: id, Direction: provider.Inbound, Format: provider.FormatPDF})
	if err != nil || !bytes.Equal(res.Data, pdf) || res.ContentType != "application/pdf" {
		t.Fatalf("PDF: %v %v", res, err)
	}
	res, err = c.Fetch(ctx, provider.FetchQuery{UUID: id, Direction: provider.Inbound, Format: provider.FormatHTML})
	if err != nil || !bytes.Equal(res.Data, html) {
		t.Fatalf("HTML: %v %v", res, err)
	}
	res, err = c.Fetch(ctx, provider.FetchQuery{Kind: provider.EArchive, UUID: id, Format: provider.FormatPDF})
	if err != nil || !bytes.Equal(res.Data, pdf) || f.last("SearchArchiveInvoice").get("IsPDFIncluded") != "true" {
		t.Fatalf("e-Arşiv PDF: %v %v", res, err)
	}
	if _, err := c.Fetch(ctx, provider.FetchQuery{Kind: provider.EArchive, UUID: id, Format: provider.FormatHTML}); !errors.Is(err, provider.ErrUnsupported) {
		t.Errorf("e-Arşiv HTML: %v", err)
	}
	if _, err := c.Fetch(ctx, provider.FetchQuery{Kind: provider.EArchive, UUID: id}); !errors.Is(err, provider.ErrNotFound) {
		t.Errorf("missing InvoiceXml: %v", err)
	}
	if _, err := c.Fetch(ctx, provider.FetchQuery{}); provider.KindOf(err) != provider.KindInvalidRequest {
		t.Errorf("empty UUID: %v", err)
	}
}

func TestEInvoiceStatus(t *testing.T) {
	for name, tc := range map[string]struct {
		status, detail string
		dir            provider.Direction
		want           provider.Status
		gib            int
		response       string
	}{
		"delivered":      {"Gonderildi", "Basariyla_Tamamlandi", provider.Outbound, provider.StatusDelivered, 1300, ""},
		"forwarded":      {"Gibe_Iletildi", "Zarf_Basariyla_Islendi", provider.Outbound, provider.StatusSent, 1200, ""},
		"no-answer":      {"Gonderildi", "Hedeften_Sistem_Yaniti_Gelmedi", provider.Outbound, provider.StatusSent, 1220, ""},
		"schematron":     {"Gonderildi", "Schematron_Kontrol_Sonucu_Hatali", provider.Outbound, provider.StatusFailed, 1150, ""},
		"receiver-fail":  {"Gonderildi", "Hedeften_Sistem_Yaniti_Basarisiz_Geldi", provider.Outbound, provider.StatusFailed, 1230, ""},
		"subject-cancel": {"Gonderildi", "Fatura_Iptale_Konu_Edildi", provider.Outbound, provider.StatusCancelled, 1235, ""},
		"queued":         {"Gonderildi", "Zarf_Kuyruga_Eklendi", provider.Outbound, provider.StatusQueued, 1000, ""},
		"pending":        {"Onay_Bekliyor", "Basariyla_Tamamlandi", provider.Outbound, provider.StatusDelivered, 1300, ""},
		"accepted":       {"Alici_Kabul_Etti", "Basariyla_Tamamlandi", provider.Outbound, provider.StatusAccepted, 1300, "KABUL"},
		"auto-accepted":  {"Otomatik_Alici_Kabul_Etti", "", provider.Outbound, provider.StatusAccepted, 0, "KABUL"},
		"rejected":       {"Alici_Reddetti", "Basariyla_Tamamlandi", provider.Outbound, provider.StatusRejected, 1300, "RED"},
		"returned":       {"Alici_Iade_Etti", "", provider.Outbound, provider.StatusRejected, 0, "IADE"},
		"gib-error":      {"Gib_Tarafinda_Hata_Olustu", "", provider.Outbound, provider.StatusFailed, 0, ""},
		"signature":      {"Imza_Bekliyor", "Zarflanmadi", provider.Outbound, provider.StatusDraft, 0, ""},
		"zipped":         {"Ziplendi", "", provider.Outbound, provider.StatusProcessing, 0, ""},
		"numeric":        {"24", "1300", provider.Outbound, provider.StatusAccepted, 1300, "KABUL"},
		"keep-previous":  {"Uygulama_Yaniti_Yollaniyor", "Basariyla_Tamamlandi", provider.Outbound, provider.StatusDelivered, 1300, ""},
		"unmapped":       {"Yeni_Bir_Durum", "", provider.Outbound, provider.StatusUnknown, 0, ""},
		"in-pending":     {"Onay_Bekliyor", "Basariyla_Tamamlandi", provider.Inbound, provider.StatusReceived, 1300, ""},
		"in-accepted":    {"Onaylandi", "", provider.Inbound, provider.StatusAccepted, 0, "KABUL"},
		"in-answering":   {"Uygulama_Yaniti_Hata_Aldi", "", provider.Inbound, provider.StatusReceived, 0, ""},
	} {
		t.Run(name, func(t *testing.T) {
			f, srv := newServer(t, func(m string, _ int, c call) string {
				return envelope(m, `<a:Invoices><a:Invoice><a:DetailStatus>`+tc.detail+`</a:DetailStatus><a:ETTN>`+c.get("Ettn")+
					`</a:ETTN><a:InvoiceNumber>ABC2026000000123</a:InvoiceNumber><a:Status>`+tc.status+
					`</a:Status><a:SystemResponseDescription>açıklama</a:SystemResponseDescription></a:Invoice></a:Invoices><a:Result>Success</a:Result>`)
			})
			res, err := newClient(t, srv.URL).Status(context.Background(), provider.StatusQuery{UUID: "u-1", Direction: tc.dir})
			if err != nil {
				t.Fatal(err)
			}
			if res.Status != tc.want || res.GIBCode != tc.gib || res.ResponseCode != tc.response {
				t.Fatalf("got %s gib %d resp %q, want %s %d %q (%s)", res.Status, res.GIBCode, res.ResponseCode, tc.want, tc.gib, tc.response, res.ProviderCode)
			}
			if res.UUID != "u-1" || res.ID != "ABC2026000000123" || res.Message != "açıklama" || len(res.Raw) == 0 {
				t.Fatalf("result %+v", res)
			}
			r := f.last("SearchAllInvoice")
			wantDir := "Outgoing"
			if tc.dir == provider.Inbound {
				wantDir = "Incoming"
			}
			if r.get("Ettn") != "u-1" || r.get("InvoiceDirection") != wantDir || r.get("IsXMLIncluded") != "false" {
				t.Fatalf("request %v", r.leaves)
			}
		})
	}
}

func TestEArchiveStatus(t *testing.T) {
	item := `<a:ArchiveInvoices><a:ArchiveInvoice><a:ETTN>e-1</a:ETTN><a:InvoiceNumber>EAR2026000000001</a:InvoiceNumber>` +
		`<a:ReportSendingStatus>Gib_Tarafinda_Hata_Olustu</a:ReportSendingStatus><a:Status>Fatura_Olusturuldu</a:Status></a:ArchiveInvoice></a:ArchiveInvoices>`
	for name, tc := range map[string]struct {
		cancelled bool
		want      provider.Status
	}{
		"active":    {false, provider.StatusDelivered},
		"cancelled": {true, provider.StatusCancelled},
	} {
		t.Run(name, func(t *testing.T) {
			f, srv := newServer(t, func(m string, n int, c call) string {
				if n == 0 {
					return envelope(m, item+`<a:Result>Success</a:Result>`)
				}
				if tc.cancelled {
					return envelope(m, item+`<a:Result>Success</a:Result>`)
				}
				return envelope(m, `<a:ArchiveInvoices/><a:Result>Success</a:Result>`)
			})
			res, err := newClient(t, srv.URL).Status(context.Background(), provider.StatusQuery{Kind: provider.EArchive, UUID: "e-1"})
			if err != nil {
				t.Fatal(err)
			}
			if res.Status != tc.want || res.ID != "EAR2026000000001" || res.Message == "" {
				t.Fatalf("result %+v", res)
			}
			calls := f.all("SearchArchiveInvoice")
			if len(calls) != 2 || calls[0].has("IsCancelled") || calls[1].get("IsCancelled") != "true" || calls[1].get("Ettn") != "e-1" {
				t.Fatalf("calls %d", len(calls))
			}
		})
	}
	// A cancellation already visible on the item needs no second query.
	f, srv := newServer(t, func(m string, _ int, _ call) string {
		return envelope(m, `<a:ArchiveInvoices><a:ArchiveInvoice><a:CancellationReportSendingStatus>Gibe_Gonderildi</a:CancellationReportSendingStatus><a:Status>Fatura_Olusturuldu</a:Status></a:ArchiveInvoice></a:ArchiveInvoices><a:Result>Success</a:Result>`)
	})
	res, err := newClient(t, srv.URL).Status(context.Background(), provider.StatusQuery{Kind: provider.EArchive, UUID: "e-2"})
	if err != nil || res.Status != provider.StatusCancelled || len(f.all("SearchArchiveInvoice")) != 1 {
		t.Fatalf("visible cancellation: %+v %v", res, err)
	}
	_, srv = newServer(t, func(m string, _ int, _ call) string {
		return envelope(m, `<a:ArchiveInvoices/><a:Result>Success</a:Result>`)
	})
	if _, err := newClient(t, srv.URL).Status(context.Background(), provider.StatusQuery{Kind: provider.EArchive, UUID: "x"}); !errors.Is(err, provider.ErrNotFound) {
		t.Fatalf("not found: %v", err)
	}
}

func TestCheckUser(t *testing.T) {
	notFound := false
	f, srv := newServer(t, func(m string, _ int, _ call) string {
		if m != "GetTaxPayer" {
			return ""
		}
		if notFound {
			return fixture(t, "get_tax_payer_not_found.xml")
		}
		return fixture(t, "get_tax_payer_response.xml")
	})
	c := newClient(t, srv.URL)
	u, err := c.CheckUser(context.Background(), "9876543210")
	if err != nil {
		t.Fatal(err)
	}
	sameShape(t, f.last("GetTaxPayer"), "get_tax_payer_request.xml")
	if !u.Registered || u.VKNTCKN != "9876543210" || u.Title != "Örnek Satıcı A.Ş." || len(u.Aliases) != 2 ||
		u.Aliases[0] != (provider.Alias{Name: "urn:mail:defaultpk@ornek.com.tr", Type: "PK", Kind: provider.EInvoice, CreatedAt: u.Aliases[0].CreatedAt}) ||
		u.Aliases[1].Type != "GB" || u.Aliases[0].CreatedAt.Year() != 2020 {
		t.Fatalf("user %+v", u)
	}
	notFound = true
	u, err = c.CheckUser(context.Background(), "5555555555")
	if err != nil || u.Registered || u.VKNTCKN != "5555555555" {
		t.Fatalf("not registered: %+v %v", u, err)
	}
	n := len(f.all("GetTaxPayer"))
	if _, err := c.CheckUser(context.Background(), " "); provider.KindOf(err) != provider.KindInvalidRequest || len(f.all("GetTaxPayer")) != n {
		t.Fatalf("empty VKN must be refused without a call: %v", err)
	}
}

func TestVendorsAndViewerLink(t *testing.T) {
	f, srv := newServer(t, func(m string, _ int, _ call) string {
		switch m {
		case "GetCompanyVendor":
			return envelope(m, `<a:ErrorMessage i:nil="true"/><a:Result>Success</a:Result><a:Vendors><a:CompanyVendor><a:companyVendorName>Şube 1</a:companyVendorName><a:companyVendorNumber>B01</a:companyVendorNumber></a:CompanyVendor></a:Vendors>`)
		case "GetDocumentViewerLink":
			return envelope(m, `<a:DocumentViewerLink>http://viewer.example:80/doc?id=1</a:DocumentViewerLink><a:ErrorMessage i:nil="true"/><a:Result>Success</a:Result>`)
		}
		return ""
	})
	c := newClient(t, srv.URL)
	v, err := c.Vendors(context.Background())
	if err != nil || len(v) != 1 || v[0] != (isnet.Vendor{Number: "B01", Name: "Şube 1"}) {
		t.Fatalf("vendors %+v %v", v, err)
	}
	sameShape(t, f.last("GetCompanyVendor"), "get_company_vendor_request.xml")
	link, err := c.DocumentViewerLink(context.Background(), provider.EInvoice, provider.Inbound, "f3b1c2d4-0000-4a5b-9c8d-123456789abc")
	if err != nil || link != "http://viewer.example:80/doc?id=1" {
		t.Fatalf("link %q %v", link, err)
	}
	sameShape(t, f.last("GetDocumentViewerLink"), "document_viewer_link_request.xml")
	if _, err := c.DocumentViewerLink(context.Background(), provider.EArchive, "", "x"); err != nil {
		t.Fatal(err)
	}
	if r := f.last("GetDocumentViewerLink"); r.get("InvoiceDocumentType") != "EArchiveInvoice" || r.get("InvoiceDirection") != "Outgoing" {
		t.Fatalf("e-Arşiv link request %v", r.leaves)
	}
}

func TestErrors(t *testing.T) {
	_, srv := newServer(t, func(m string, _ int, _ call) string {
		switch m {
		case "SearchAllInvoice":
			return envelope(m, `<a:ErrorMessage>örnek hata metni</a:ErrorMessage><a:Result>Failed</a:Result>`)
		case "SearchArchiveInvoice":
			return fixture(t, "internal_service_fault.xml")
		}
		return ""
	})
	c := newClient(t, srv.URL)
	_, err := c.ListInbox(context.Background(), provider.ListQuery{})
	var pe *provider.Error
	if !errors.As(err, &pe) || pe.Code != "Failed" || pe.Message != "örnek hata metni" || pe.Kind != provider.KindUnknown ||
		pe.Op != "SearchAllInvoice" || len(pe.Raw) == 0 || isnet.IsIPNotAllowed(err) {
		t.Fatalf("Result=Failed: %#v", err)
	}
	_, err = c.ListOutbox(context.Background(), provider.ListQuery{Kind: provider.EArchive})
	if !errors.As(err, &pe) || pe.Kind != provider.KindTransport || pe.Code != "a:InternalServiceFault" || pe.HTTPStatus != 500 {
		t.Fatalf("fault: %#v", err)
	}
}
