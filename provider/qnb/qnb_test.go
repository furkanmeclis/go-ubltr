package qnb_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/furkanmeclis/go-ubltr/provider"
	"github.com/furkanmeclis/go-ubltr/provider/providertest"
	"github.com/furkanmeclis/go-ubltr/provider/qnb"
)

const (
	nsConn = "http://service.connector.uut.cs.com.tr/"
	nsArc  = "http://service.earsiv.uut.cs.com.tr/"
	wsse   = "http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-secext-1.0.xsd"
)

const invoiceXML = `<?xml version="1.0" encoding="UTF-8"?>
<Invoice xmlns="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2"
 xmlns:cac="urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2"
 xmlns:cbc="urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2">
 <cbc:ProfileID>TICARIFATURA</cbc:ProfileID>
 <cbc:ID>ABC2026000000001</cbc:ID>
 <cbc:UUID>25ADF33C-C065-4AB9-A0F2-30A72419C607</cbc:UUID>
 <cbc:InvoiceTypeCode>SATIS</cbc:InvoiceTypeCode>
 <cac:AccountingCustomerParty><cac:Party><cac:PartyIdentification><cbc:ID schemeID="VKN">2222222222</cbc:ID></cac:PartyIdentification></cac:Party></cac:AccountingCustomerParty>
</Invoice>`

// request is a parsed SOAP request.
type request struct {
	Op, NS   string
	User     string
	Fields   map[string][]string
	Unqualif bool
}

func (r request) get(k string) string {
	if v := r.Fields[k]; len(v) > 0 {
		return v[0]
	}
	return ""
}

// parse reads the operation element and its leaf children (by local name).
func parse(t *testing.T, body []byte) request {
	t.Helper()
	r := request{Fields: map[string][]string{}, Unqualif: true}
	dec := xml.NewDecoder(bytes.NewReader(body))
	var stack []xml.Name
	inBody, opDepth := false, -1
	var text strings.Builder
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("bad request xml: %v\n%s", err, body)
		}
		switch v := tok.(type) {
		case xml.StartElement:
			stack = append(stack, v.Name)
			text.Reset()
			if v.Name.Local == "Body" {
				inBody = true
				continue
			}
			if inBody && opDepth < 0 {
				r.Op, r.NS, opDepth = v.Name.Local, v.Name.Space, len(stack)
				continue
			}
			if opDepth > 0 && v.Name.Space != "" {
				r.Unqualif = false
			}
		case xml.CharData:
			text.Write(v)
		case xml.EndElement:
			n := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if n.Space == wsse && n.Local == "Username" {
				r.User = strings.TrimSpace(text.String())
			}
			if opDepth > 0 && len(stack) >= opDepth {
				r.Fields[n.Local] = append(r.Fields[n.Local], strings.TrimSpace(text.String()))
			}
			text.Reset()
		}
	}
	return r
}

type fakeServer struct {
	t     *testing.T
	mu    sync.Mutex
	calls []request
	reply map[string]string
}

func newServer(t *testing.T, reply map[string]string) (*fakeServer, *httptest.Server) {
	f := &fakeServer{t: t, reply: reply}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		req := parse(t, body)
		f.mu.Lock()
		f.calls = append(f.calls, req)
		f.mu.Unlock()
		if !req.Unqualif {
			t.Errorf("%s: child elements must be unqualified:\n%s", req.Op, body)
		}
		if req.User == "" {
			t.Errorf("%s: missing WS-Security UsernameToken", req.Op)
		}
		out, ok := reply[req.Op]
		if !ok {
			t.Errorf("unexpected operation %s", req.Op)
			w.WriteHeader(500)
			return
		}
		w.Header().Set("Content-Type", "text/xml; charset=utf-8")
		if strings.Contains(out, "<S:Fault>") {
			w.WriteHeader(500)
		}
		_, _ = io.WriteString(w, `<S:Envelope xmlns:S="http://schemas.xmlsoap.org/soap/envelope/"><S:Body>`+out+`</S:Body></S:Envelope>`)
	}))
	t.Cleanup(srv.Close)
	return f, srv
}

func (f *fakeServer) last(op string) request {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := len(f.calls) - 1; i >= 0; i-- {
		if f.calls[i].Op == op {
			return f.calls[i]
		}
	}
	f.t.Fatalf("no %s call", op)
	return request{}
}

func newClient(t *testing.T, url string) *qnb.Client {
	t.Helper()
	c, err := qnb.New(qnb.Config{
		Username: "ws", Password: "pw", VKN: "3250566851", ERPCode: "ERP1",
		EArchiveUsername: "ws.portal", EArchivePassword: "pw2",
		ConnectorURL: url, EArchiveURL: url,
	})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func zipB64(t *testing.T, name string, data []byte) string {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create(name)
	_, _ = w.Write(data)
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

// Response bodies follow the samples in the QNB API documentation.
const durumOK = `<ns2:gidenBelgeDurumSorgulaExtResponse xmlns:ns2="http://service.connector.uut.cs.com.tr/">
<return xsi:type="ns2:gidenBelgeDurumv5" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
<alimTarihi>20220617214636473</alimTarihi><belgeNo>SAL2022090578078</belgeNo><durum>3</durum>
<ettn>25ADF33C-C065-4AB9-A0F2-30A72419C607</ettn><gonderimCevabiDetayi/><gonderimCevabiKodu>1300</gonderimCevabiKodu>
<gonderimDurumu>4</gonderimDurumu><olusturulmaTarihi>20220617214644893</olusturulmaTarihi><yanitDetayi/>
<yanitDurumu>2</yanitDurumu><ulastiMi>true</ulastiMi><yenidenGonderilebilirMi>false</yenidenGonderilebilirMi>
<yerelBelgeOid>3ul4dvqhmd1095</yerelBelgeOid></return></ns2:gidenBelgeDurumSorgulaExtResponse>`

func TestCapabilitiesAndCodes(t *testing.T) {
	c := newClient(t, "http://127.0.0.1:1")
	providertest.CheckCapabilities(t, c)
	providertest.CheckCodeTable(t, qnb.Codes)
	if !provider.Supports(c, provider.EArchive, provider.OpCancelEArchive) || provider.Supports(c, provider.EInvoice, provider.OpRespond) {
		t.Fatal("capabilities mismatch")
	}
	p, err := provider.Open("qnb", provider.Config{Username: "u", Password: "p", VKN: "1", Test: true})
	if err != nil || p.Name() != "qnb" {
		t.Fatalf("Open: %v", err)
	}
	if _, err := qnb.New(qnb.Config{Username: "u", Password: "p", VKN: "1"}); err == nil {
		t.Fatal("live without endpoints must fail")
	}
}

func TestSendEInvoice(t *testing.T) {
	f, srv := newServer(t, map[string]string{
		"belgeGonderExt": `<ns2:belgeGonderExtResponse xmlns:ns2="` + nsConn + `"><belgeOid>3ul4dvqhmd1095</belgeOid></ns2:belgeGonderExtResponse>`,
	})
	c := newClient(t, srv.URL)
	res, err := c.Send(context.Background(), &provider.Document{XML: []byte(invoiceXML), ReceiverAlias: "urn:mail:defaultpk@firma.com.tr"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != provider.StatusQueued || res.ProviderCode != "3ul4dvqhmd1095" || res.UUID != "25ADF33C-C065-4AB9-A0F2-30A72419C607" {
		t.Fatalf("result %+v", res)
	}
	r := f.last("belgeGonderExt")
	if r.NS != nsConn || r.User != "ws" {
		t.Fatalf("ns %q user %q", r.NS, r.User)
	}
	want := map[string]string{
		"vergiTcKimlikNo": "3250566851", "belgeTuru": "FATURA_UBL", "belgeNo": "25ADF33C-C065-4AB9-A0F2-30A72419C607",
		"mimeType": "application/xml", "belgeVersiyon": "1.0", "erpKodu": "ERP1", "alanEtiket": "urn:mail:defaultpk@firma.com.tr",
	}
	for k, v := range want {
		if r.get(k) != v {
			t.Errorf("%s = %q, want %q", k, r.get(k), v)
		}
	}
	veri, _ := base64.StdEncoding.DecodeString(r.get("veri"))
	if string(veri) != invoiceXML {
		t.Error("veri is not the base64 XML")
	}
	if h := r.get("belgeHash"); len(h) != 32 || strings.ToUpper(h) != h {
		t.Errorf("belgeHash %q: want upper-case MD5 hex", h)
	}
}

func TestStatusMapping(t *testing.T) {
	for name, tc := range map[string]struct {
		durum, gonderim, kod, yanit string
		want                        provider.Status
		gib                         int
	}{
		"received":  {"1", "-1", "0", "-1", provider.StatusQueued, 0},
		"error":     {"2", "-1", "0", "-1", provider.StatusFailed, 0},
		"queued":    {"3", "-1", "0", "-1", provider.StatusProcessing, 0},
		"sent":      {"3", "2", "0", "-1", provider.StatusSent, 0},
		"gib-1220":  {"3", "3", "1220", "0", provider.StatusSent, 1220},
		"delivered": {"3", "4", "1300", "0", provider.StatusDelivered, 1300},
		"accepted":  {"3", "4", "1300", "2", provider.StatusAccepted, 1300},
		"rejected":  {"3", "4", "1300", "1", provider.StatusRejected, 1300},
		"gib-1163":  {"3", "3", "1163", "-1", provider.StatusFailed, 1163},
		"cancelled": {"3", "-2", "0", "-1", provider.StatusCancelled, 0},
	} {
		t.Run(name, func(t *testing.T) {
			body := strings.NewReplacer("<durum>3<", "<durum>"+tc.durum+"<", "<gonderimDurumu>4<", "<gonderimDurumu>"+tc.gonderim+"<",
				"<gonderimCevabiKodu>1300<", "<gonderimCevabiKodu>"+tc.kod+"<", "<yanitDurumu>2<", "<yanitDurumu>"+tc.yanit+"<",
				"<ulastiMi>true<", "<ulastiMi>false<").Replace(durumOK)
			f, srv := newServer(t, map[string]string{"gidenBelgeDurumSorgulaExt": body})
			c := newClient(t, srv.URL)
			res, err := c.Status(context.Background(), provider.StatusQuery{Kind: provider.EInvoice, UUID: "25ADF33C-C065-4AB9-A0F2-30A72419C607"})
			if err != nil {
				t.Fatal(err)
			}
			if res.Status != tc.want || res.GIBCode != tc.gib {
				t.Fatalf("status %s gib %d, want %s %d (%s)", res.Status, res.GIBCode, tc.want, tc.gib, res.ProviderCode)
			}
			r := f.last("gidenBelgeDurumSorgulaExt")
			if r.get("belgeNoTipi") != "YEREL" || r.get("belgeNo") != "25ADF33C-C065-4AB9-A0F2-30A72419C607" || r.get("vergiTcKimlikNo") != "3250566851" {
				t.Fatalf("request %+v", r.Fields)
			}
			if len(res.Raw) == 0 {
				t.Fatal("raw missing")
			}
		})
	}
}

func TestStatusFallsBackToETTN(t *testing.T) {
	n := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		req := parse(t, body)
		n++
		if req.get("belgeNoTipi") == "YEREL" {
			w.WriteHeader(500)
			_, _ = io.WriteString(w, `<S:Envelope xmlns:S="http://schemas.xmlsoap.org/soap/envelope/"><S:Body><S:Fault><faultcode>S:Server</faultcode><faultstring>X ettn'li belge bulunamadı</faultstring></S:Fault></S:Body></S:Envelope>`)
			return
		}
		_, _ = io.WriteString(w, `<S:Envelope xmlns:S="http://schemas.xmlsoap.org/soap/envelope/"><S:Body>`+durumOK+`</S:Body></S:Envelope>`)
	}))
	defer srv.Close()
	res, err := newClient(t, srv.URL).Status(context.Background(), provider.StatusQuery{Kind: provider.EInvoice, UUID: "u"})
	if err != nil || n != 2 || res.Status != provider.StatusAccepted || res.ResponseCode != "KABUL" {
		t.Fatalf("res %+v err %v calls %d", res, err, n)
	}
}

func TestFaultClassification(t *testing.T) {
	for text, kind := range map[string]provider.ErrorKind{
		"username.pwd.mismatch":                 provider.KindAuth,
		"Hash hatası":                           provider.KindValidation,
		"1172 - TPS_POSTA_KUTUSU_YETKI_YOK":     provider.KindPermission,
		"bilinmeyen bir hata":                   provider.KindTransport,
		"5544332211 ettn'li belge bulunamadı !": provider.KindNotFound,
	} {
		_, srv := newServer(t, map[string]string{
			"belgeGonderExt": `<S:Fault><faultcode>S:Server</faultcode><faultstring>` + text + `</faultstring><detail><ns2:Exception xmlns:ns2="` + nsConn + `"><message>` + text + `</message></ns2:Exception></detail></S:Fault>`,
		})
		_, err := newClient(t, srv.URL).Send(context.Background(), &provider.Document{XML: []byte(invoiceXML)})
		if provider.KindOf(err) != kind {
			t.Errorf("%q: kind %s, want %s (%v)", text, provider.KindOf(err), kind, err)
		}
	}
}

func TestListInboxAndMarkTaken(t *testing.T) {
	item := func(sira, ettn string) string {
		return `<return xsi:type="ns2:belgev6" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"><belgeNo>AAA2017000000080</belgeNo>
<belgeSiraNo>` + sira + `</belgeSiraNo><belgeTarihi>20170311</belgeTarihi><belgeTuru>FATURA</belgeTuru><ettn>` + ettn + `</ettn>
<gonderenEtiket>urn:mail:testgb@cs.com.tr</gonderenEtiket><gonderenVknTckn>1234567802</gonderenVknTckn><alanEtiket>urn:mail:test@test.com.tr</alanEtiket>
<aliciUnvan>Test A.Ş</aliciUnvan><belgeVersiyon>1.0</belgeVersiyon><saticiUnvan>Satıcı A.Ş.</saticiUnvan><zarfId>06840CFC-25D6-4794-BEA1-420CE078C760</zarfId>
<odenecekTutar>77880.00</odenecekTutar><odenecekTutarDovizCinsi>TRY</odenecekTutarDovizCinsi><arsivlenmis>1</arsivlenmis>
<faturaGelisTarihi>20170208160730818</faturaGelisTarihi><profileId>TEMELFATURA</profileId></return>`
	}
	var items strings.Builder
	for i := 0; i < 100; i++ {
		items.WriteString(item(strings.Repeat("1", 1)+string(rune('0'+i%10)), "E"+string(rune('A'+i%26))))
	}
	f, srv := newServer(t, map[string]string{
		"gelenBelgeleriListeleExt": `<ns2:gelenBelgeleriListeleExtResponse xmlns:ns2="` + nsConn + `">` + item("1010", "22056A27-D326-4FE0-A538-F775F36A28BA") + items.String() + `</ns2:gelenBelgeleriListeleExtResponse>`,
		"belgelerAlindi":           `<ns2:belgelerAlindiResponse xmlns:ns2="` + nsConn + `"/>`,
	})
	c := newClient(t, srv.URL)
	page, err := c.ListInbox(context.Background(), provider.ListQuery{OnlyNew: true, Cursor: "1000"})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 101 || page.Next == "" {
		t.Fatalf("items %d next %q", len(page.Items), page.Next)
	}
	d := page.Items[0]
	if d.UUID != "22056A27-D326-4FE0-A538-F775F36A28BA" || d.IssueDate != "2017-03-11" || d.SenderVKN != "1234567802" ||
		d.Profile != "TEMELFATURA" || d.Payable != "77880.00" || d.Currency != "TRY" || d.Status != provider.StatusReceived ||
		d.CreatedAt.Year() != 2017 || d.SenderName != "Satıcı A.Ş." {
		t.Fatalf("item %+v", d)
	}
	r := f.last("gelenBelgeleriListeleExt")
	if r.get("sonAlinanBelgeSiraNumarasi") != "1000" || r.get("belgeTuru") != "FATURA" || r.get("belgelerAlindiMi") != "false" || r.get("donusTipiVersiyon") != "6.0" {
		t.Fatalf("request %+v", r.Fields)
	}
	limited, err := c.ListInbox(context.Background(), provider.ListQuery{Limit: 1})
	if err != nil || len(limited.Items) != 1 || limited.Next != "1010" {
		t.Fatalf("limit: %+v %v", limited, err)
	}
	if err := c.MarkTaken(context.Background(), provider.Inbound, "a", "b"); err != nil {
		t.Fatal(err)
	}
	if got := f.last("belgelerAlindi").Fields["ettn"]; len(got) != 2 || got[1] != "b" {
		t.Fatalf("ettn %v", got)
	}
	if !errors.Is(c.MarkTaken(context.Background(), provider.Outbound, "a"), provider.ErrUnsupported) {
		t.Fatal("outbound MarkTaken must be unsupported")
	}
}

func TestFetch(t *testing.T) {
	pdf := []byte("%PDF-1.4 test")
	f, srv := newServer(t, map[string]string{
		"gelenBelgeleriIndirExt":    `<ns2:gelenBelgeleriIndirExtResponse xmlns:ns2="` + nsConn + `"><return>` + zipB64(t, "a.xml", []byte(invoiceXML)) + `</return></ns2:gelenBelgeleriIndirExtResponse>`,
		"gidenBelgeDurumSorgulaExt": durumOK,
		"gidenBelgeleriIndir":       `<ns2:gidenBelgeleriIndirResponse xmlns:ns2="` + nsConn + `"><return>` + zipB64(t, "b.pdf", pdf) + `</return></ns2:gidenBelgeleriIndirResponse>`,
	})
	c := newClient(t, srv.URL)
	in, err := c.Fetch(context.Background(), provider.FetchQuery{Kind: provider.EInvoice, Direction: provider.Inbound, UUID: "587d1170", Format: provider.FormatUBL})
	if err != nil || string(in.Data) != invoiceXML || in.ContentType != "application/xml" {
		t.Fatalf("inbound %v %v", in, err)
	}
	if r := f.last("gelenBelgeleriIndirExt"); r.get("ettn") != "587d1170" || r.get("belgeFormati") != "UBL" || r.get("belgeTuru") != "FATURA" {
		t.Fatalf("request %+v", r.Fields)
	}
	out, err := c.Fetch(context.Background(), provider.FetchQuery{Kind: provider.EInvoice, Direction: provider.Outbound, UUID: "25ADF33C", Format: provider.FormatPDF})
	if err != nil || !bytes.Equal(out.Data, pdf) {
		t.Fatalf("outbound %v %v", out, err)
	}
	if r := f.last("gidenBelgeleriIndir"); r.get("belgeOidListesi") != "3ul4dvqhmd1095" || r.get("belgeFormati") != "PDF" {
		t.Fatalf("request %+v", r.Fields)
	}
}

func TestCheckUserAndCredits(t *testing.T) {
	_, srv := newServer(t, map[string]string{
		"efaturaKullaniciBilgisi": `<ns2:efaturaKullaniciBilgisiResponse xmlns:ns2="` + nsConn + `"><return><etiket>urn:mail:defaultpk@example.com.tr</etiket>
<kamuKurulusu>false</kamuKurulusu><kayitZamani>20240101120000</kayitZamani><unvan>Örnek Firma A.Ş.</unvan><vergiTcKimlikNo>1234567890</vergiTcKimlikNo></return></ns2:efaturaKullaniciBilgisiResponse>`,
		"kontorBilgisiGetir": `<ns2:kontorBilgisiGetirResponse xmlns:ns2="` + nsConn + `"><return><kalan>120</kalan><kontorBirimi>ADET</kontorBirimi><kontorTipi>FATURA</kontorTipi><toplamAlinan>500</toplamAlinan><vknTckn>3250566851</vknTckn></return></ns2:kontorBilgisiGetirResponse>`,
	})
	c := newClient(t, srv.URL)
	u, err := c.CheckUser(context.Background(), "1234567890")
	if err != nil || !u.Registered || u.Title != "Örnek Firma A.Ş." || len(u.Aliases) != 1 || u.Aliases[0].Name != "urn:mail:defaultpk@example.com.tr" || u.Aliases[0].CreatedAt.Year() != 2024 {
		t.Fatalf("user %+v %v", u, err)
	}
	cr, err := c.Credits(context.Background())
	if err != nil || cr.Remaining != 120 || cr.Used != 380 {
		t.Fatalf("credits %+v %v", cr, err)
	}
}

const earsivOK = `<output><belgeFormati>HTML</belgeFormati><belgeIcerigi>PGh0bWw+PC9odG1sPg==</belgeIcerigi></output><return><resultCode>AE00000</resultCode><resultExtra>
<entry><key xsi:type="xs:string" xmlns:xs="http://www.w3.org/2001/XMLSchema" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">islemID</key><value xsi:type="xs:string" xmlns:xs="http://www.w3.org/2001/XMLSchema" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">1303d6ed-db9e-4465-b48e-9ec6791a3bcb</value></entry>
<entry><key>uuid</key><value>1303d6ed-db9e-4465-b48e-9ec6791a3bcb</value></entry>
<entry><key>faturaNo</key><value>TTT2026000000001</value></entry></resultExtra><resultText>İşlem başarılı.</resultText></return>`

func TestEArchive(t *testing.T) {
	f, srv := newServer(t, map[string]string{
		"faturaOlusturExt": `<ns2:faturaOlusturExtResponse xmlns:ns2="` + nsArc + `">` + earsivOK + `</ns2:faturaOlusturExtResponse>`,
		"faturaSorgulaExt": `<ns2:faturaSorgulaExtResponse xmlns:ns2="` + nsArc + `">` + strings.Replace(earsivOK, "</resultExtra>", `<entry><key>iptalTarihi</key><value>20260102</value></entry></resultExtra>`, 1) + `</ns2:faturaSorgulaExtResponse>`,
		"faturaIptalEt":    `<ns2:faturaIptalEtResponse xmlns:ns2="` + nsArc + `"><return><resultCode>AE00001</resultCode><resultExtra/><resultText>Fatura bulunamadı</resultText></return></ns2:faturaIptalEtResponse>`,
	})
	c := newClient(t, srv.URL)
	doc := &provider.Document{XML: []byte(strings.Replace(invoiceXML, "TICARIFATURA", "EARSIVFATURA", 1)), ID: ""}
	doc.UUID = "1303d6ed-db9e-4465-b48e-9ec6791a3bcb"
	res, err := c.Send(context.Background(), doc)
	if err != nil {
		t.Fatal(err)
	}
	if res.ID != "TTT2026000000001" || res.Status != provider.StatusDelivered || res.ProviderCode != "AE00000" {
		t.Fatalf("send %+v", res)
	}
	r := f.last("faturaOlusturExt")
	if r.NS != nsArc || r.User != "ws.portal" || r.get("belgeFormati") != "UBL" {
		t.Fatalf("request %+v user %q", r.Fields, r.User)
	}
	var in map[string]any
	if err := json.Unmarshal([]byte(r.get("input")), &in); err != nil {
		t.Fatal(err)
	}
	if in["islemId"] != doc.UUID || in["vkn"] != "3250566851" || in["sube"] != "DFLT" || in["kasa"] != "DFLT" || in["erpKodu"] != "ERP1" || in["donenBelgeFormati"] != "9" {
		t.Fatalf("input %v", in)
	}
	st, err := c.Status(context.Background(), provider.StatusQuery{Kind: provider.EArchive, UUID: doc.UUID})
	if err != nil || st.Status != provider.StatusCancelled || st.ID != "TTT2026000000001" {
		t.Fatalf("status %+v %v", st, err)
	}
	fr, err := c.Fetch(context.Background(), provider.FetchQuery{Kind: provider.EArchive, UUID: doc.UUID, Format: provider.FormatHTML})
	if err != nil || string(fr.Data) != "<html></html>" {
		t.Fatalf("fetch %v %v", fr, err)
	}
	if err := json.Unmarshal([]byte(f.last("faturaSorgulaExt").get("input")), &in); err != nil || in["donenBelgeFormati"] != "2" || in["faturaUuid"] != doc.UUID {
		t.Fatalf("sorgula input %v", in)
	}
	err = c.CancelEArchive(context.Background(), doc.UUID, "")
	var pe *provider.Error
	if !errors.As(err, &pe) || pe.Code != "AE00001" || pe.Message != "Fatura bulunamadı" {
		t.Fatalf("cancel err %v", err)
	}
}
