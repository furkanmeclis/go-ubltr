package sovos_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/furkanmeclis/go-ubltr/provider"
	"github.com/furkanmeclis/go-ubltr/provider/providertest"
	"github.com/furkanmeclis/go-ubltr/provider/sovos"
)

const (
	nsEInv   = "http:/fitcons.com/eInvoice/"
	nsArc    = "http://fitcons.com/earchive/invoice"
	nsCancel = "http://fitcons.com/earchive/invoicecancellation"
	envOpen  = `<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"><s:Body>`
	envClose = `</s:Body></s:Envelope>`
)

const invoiceXML = `<?xml version="1.0" encoding="UTF-8"?>
<Invoice xmlns="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2"
 xmlns:cac="urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2"
 xmlns:cbc="urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2">
 <cbc:ProfileID>TICARIFATURA</cbc:ProfileID>
 <cbc:ID>YN02015000000024</cbc:ID>
 <cbc:UUID>F47AC10B-58CC-4372-A567-0E02B2C3D479</cbc:UUID>
 <cbc:InvoiceTypeCode>SATIS</cbc:InvoiceTypeCode>
 <cac:AccountingCustomerParty><cac:Party><cac:PartyIdentification><cbc:ID schemeID="VKN">2222222222</cbc:ID></cac:PartyIdentification></cac:Party></cac:AccountingCustomerParty>
 <cac:LegalMonetaryTotal><cbc:LineExtensionAmount currencyID="TRY">206.19</cbc:LineExtensionAmount><cbc:TaxExclusiveAmount currencyID="TRY">206.19</cbc:TaxExclusiveAmount></cac:LegalMonetaryTotal>
</Invoice>`

type request struct {
	Action, Op, NS string
	ChildNS        map[string]bool
	Fields         map[string][]string
}

func (r request) get(k string) string {
	if v := r.Fields[k]; len(v) > 0 {
		return v[0]
	}
	return ""
}

func parse(t *testing.T, body []byte) request {
	t.Helper()
	r := request{Fields: map[string][]string{}, ChildNS: map[string]bool{}}
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
			t.Fatalf("bad request: %v\n%s", err, body)
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
			if opDepth > 0 {
				r.ChildNS[v.Name.Space] = true
			}
		case xml.CharData:
			text.Write(v)
		case xml.EndElement:
			n := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if opDepth > 0 && len(stack) >= opDepth {
				r.Fields[n.Local] = append(r.Fields[n.Local], strings.TrimSpace(text.String()))
			}
			text.Reset()
		}
	}
	return r
}

type fake struct {
	t     *testing.T
	mu    sync.Mutex
	calls []request
}

// newServer answers by SOAPAction. Bodies starting with "FAULT" are sent
// as SOAP faults with HTTP 500.
func newServer(t *testing.T, reply map[string]string) (*fake, *httptest.Server) {
	f := &fake{t: t}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u, p, ok := r.BasicAuth(); !ok || u != "ws" || p != "pw" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		body, _ := io.ReadAll(r.Body)
		req := parse(t, body)
		req.Action = strings.Trim(r.Header.Get("SOAPAction"), `"`)
		f.mu.Lock()
		f.calls = append(f.calls, req)
		f.mu.Unlock()
		out, ok := reply[req.Action]
		if !ok {
			t.Errorf("unexpected action %q", req.Action)
			w.WriteHeader(500)
			return
		}
		if rest, isFault := strings.CutPrefix(out, "FAULT"); isFault {
			w.WriteHeader(500)
			out = rest
		}
		_, _ = io.WriteString(w, envOpen+out+envClose)
	}))
	t.Cleanup(srv.Close)
	return f, srv
}

func (f *fake) last(action string) request {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := len(f.calls) - 1; i >= 0; i-- {
		if f.calls[i].Action == action {
			return f.calls[i]
		}
	}
	f.t.Fatalf("no %s call", action)
	return request{}
}

func newClient(t *testing.T, url string) *sovos.Client {
	t.Helper()
	c, err := sovos.New(sovos.Config{
		Username: "ws", Password: "pw", VKN: "1234567801",
		SenderAlias: "urn:mail:defaultgb@fitcons.com", PostboxAlias: "urn:mail:defaultpk@fitcons.com",
		EInvoiceURL: url, EArchiveURL: url,
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

func TestCapabilitiesAndCodes(t *testing.T) {
	c := newClient(t, "http://127.0.0.1:1")
	providertest.CheckCapabilities(t, c)
	providertest.CheckCodeTable(t, sovos.Codes)
	p, err := provider.Open("sovos", provider.Config{Username: "u", Password: "p", VKN: "1", Test: true})
	if err != nil || p.Name() != "sovos" {
		t.Fatalf("Open: %v", err)
	}
	if _, err := sovos.New(sovos.Config{Username: "u", Password: "p", VKN: "1"}); err == nil {
		t.Fatal("live without endpoints must fail")
	}
}

func TestSendEInvoice(t *testing.T) {
	f, srv := newServer(t, map[string]string{
		"sendUBL": `<sendUBLResponse xmlns="http:/fitcons.com/eInvoice/"><Response><EnvUUID>3b3af8b3-14c2-44f0-82bc-4fddbc138dfc</EnvUUID>
<UUID>F47AC10B-58CC-4372-A567-0E02B2C3D479</UUID><ID>GIB2021000000001</ID><CustInvID>A5000512113</CustInvID></Response></sendUBLResponse>`,
	})
	c := newClient(t, srv.URL)
	res, err := c.Send(context.Background(), &provider.Document{XML: []byte(invoiceXML), ReceiverAlias: "urn:mail:defaultpk@fitcons.com"})
	if err != nil {
		t.Fatal(err)
	}
	if res.EnvelopeUUID != "3b3af8b3-14c2-44f0-82bc-4fddbc138dfc" || res.ID != "GIB2021000000001" || res.Status != provider.StatusQueued {
		t.Fatalf("result %+v", res)
	}
	r := f.last("sendUBL")
	if r.NS != nsEInv || len(r.ChildNS) != 1 || !r.ChildNS[nsEInv] {
		t.Fatalf("namespaces op %q children %v", r.NS, r.ChildNS)
	}
	if r.get("DocType") != "INVOICE" || r.get("VKN_TCKN") != "1234567801" || r.get("SenderIdentifier") != "urn:mail:defaultgb@fitcons.com" || r.get("ReceiverIdentifier") != "urn:mail:defaultpk@fitcons.com" {
		t.Fatalf("fields %+v", r.Fields)
	}
	zipped, _ := base64.StdEncoding.DecodeString(r.get("DocData"))
	zr, err := zip.NewReader(bytes.NewReader(zipped), int64(len(zipped)))
	if err != nil || len(zr.File) != 1 || zr.File[0].Name != "F47AC10B-58CC-4372-A567-0E02B2C3D479.xml" {
		t.Fatalf("zip: %v", err)
	}
	if _, err := c.Send(context.Background(), &provider.Document{XML: []byte(invoiceXML)}); !errors.Is(err, provider.ErrInvalidRequest) {
		t.Fatalf("missing receiver alias: %v", err)
	}
}

func TestFaults(t *testing.T) {
	for code, kind := range map[string]provider.ErrorKind{
		"3011": provider.KindDuplicate,
		"1101": provider.KindRecipient,
		"1300": provider.KindRateLimit,
		"1020": provider.KindValidation,
		"9999": provider.KindUnknown,
	} {
		_, srv := newServer(t, map[string]string{
			"sendUBL": `FAULT<s:Fault><faultcode>s:Client</faultcode><faultstring>ProcessingFault</faultstring><detail><ProcessingFault xmlns="http:/fitcons.com/eInvoice/"><Code>` + code + `</Code><Message>detay</Message></ProcessingFault></detail></s:Fault>`,
		})
		_, err := newClient(t, srv.URL).Send(context.Background(), &provider.Document{XML: []byte(invoiceXML), ReceiverAlias: "pk"})
		var pe *provider.Error
		if !errors.As(err, &pe) || pe.Kind != kind || pe.Code != code || pe.Message != "detay" {
			t.Errorf("code %s: %v", code, err)
		}
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusUnauthorized) }))
	defer srv.Close()
	_, err := newClient(t, srv.URL).Status(context.Background(), provider.StatusQuery{UUID: "x"})
	if !errors.Is(err, provider.ErrAuth) {
		t.Fatalf("401: %v", err)
	}
}

func TestStatusEInvoice(t *testing.T) {
	for code, want := range map[string]provider.Status{
		"1000": provider.StatusQueued, "1100": provider.StatusProcessing, "1220": provider.StatusSent,
		"1300": provider.StatusDelivered, "1163": provider.StatusFailed, "1230": provider.StatusFailed,
	} {
		f, srv := newServer(t, map[string]string{
			"getUBLList": `<getUBLListResponse xmlns="http:/fitcons.com/eInvoice/"><UBLList><UUID>F47AC10B-58CC-4372-A567-0E02B2C3D479</UUID>
<Identifier>urn:mail:defaultpk@fitcons.com</Identifier><VKN_TCKN>2222222222</VKN_TCKN><EnvUUID>7b99499b-7cf5-4a69-a407-22dab3c43d1e</EnvUUID>
<ID>YN02015000000024</ID><InsertDateTime>2015-11-12T10:00:00.000+02:00</InsertDateTime></UBLList></getUBLListResponse>`,
			"getEnvelopeStatus": `<getEnvelopeStatusResponse xmlns="http:/fitcons.com/eInvoice/"><Response><UUID>7b99499b-7cf5-4a69-a407-22dab3c43d1e</UUID>
<IssueDate>2015-11-12</IssueDate><DocumentTypeCode>SENDERENVELOPE</DocumentTypeCode><ResponseCode>` + code + `</ResponseCode><Description>açıklama</Description></Response></getEnvelopeStatusResponse>`,
		})
		res, err := newClient(t, srv.URL).Status(context.Background(), provider.StatusQuery{Kind: provider.EInvoice, UUID: "F47AC10B-58CC-4372-A567-0E02B2C3D479"})
		if err != nil {
			t.Fatal(err)
		}
		if res.Status != want || res.ProviderCode != code || res.ID != "YN02015000000024" || res.Message != "açıklama" {
			t.Errorf("%s: %+v", code, res)
		}
		if r := f.last("getUBLList"); r.get("Type") != "OUTBOUND" || r.get("DocType") != "INVOICE" || r.get("UUID") != "F47AC10B-58CC-4372-A567-0E02B2C3D479" || r.get("Identifier") != "urn:mail:defaultgb@fitcons.com" {
			t.Errorf("list request %+v", r.Fields)
		}
		if r := f.last("getEnvelopeStatus"); r.get("UUID") != "7b99499b-7cf5-4a69-a407-22dab3c43d1e" {
			t.Errorf("status request %+v", r.Fields)
		}
	}
}

func TestListInboxWindows(t *testing.T) {
	f, srv := newServer(t, map[string]string{
		"getUBLList": `<getUBLListResponse xmlns="http:/fitcons.com/eInvoice/"><UBLList><UUID>6586940a-ac02-483f-a3a5-8b4b896c23b9</UUID>
<Identifier>urn:mail:satici@x.com</Identifier><VKN_TCKN>3880718497</VKN_TCKN><EnvUUID>e1</EnvUUID><ID>ABC2026000000001</ID>
<InsertDateTime>2026-01-01T10:00:00.000+03:00</InsertDateTime></UBLList></getUBLListResponse>`,
	})
	c := newClient(t, srv.URL)
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	page, err := c.ListInbox(context.Background(), provider.ListQuery{From: from, To: from.Add(50 * time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].SenderVKN != "3880718497" || page.Items[0].Direction != provider.Inbound || page.Items[0].CreatedAt.IsZero() {
		t.Fatalf("items %+v", page.Items)
	}
	if page.Next != from.Add(24*time.Hour).Format(time.RFC3339Nano) {
		t.Fatalf("next %q", page.Next)
	}
	r := f.last("getUBLList")
	if r.get("Type") != "INBOUND" || r.get("Identifier") != "urn:mail:defaultpk@fitcons.com" || r.get("FromDate") != "2026-01-01T03:00:00.000+03:00" || r.get("ToDate") != "2026-01-02T03:00:00.000+03:00" {
		t.Fatalf("request %+v", r.Fields)
	}
	last, err := c.ListInbox(context.Background(), provider.ListQuery{From: from, To: from.Add(50 * time.Hour), Cursor: from.Add(48 * time.Hour).Format(time.RFC3339Nano)})
	if err != nil || last.Next != "" {
		t.Fatalf("last page %+v %v", last, err)
	}
	if _, err := c.ListOutbox(context.Background(), provider.ListQuery{}); err != nil {
		t.Fatal(err)
	}
	if r := f.last("getUBLList"); r.get("Type") != "OUTBOUND" || r.get("Identifier") != "urn:mail:defaultgb@fitcons.com" {
		t.Fatalf("outbox request %+v", r.Fields)
	}
	if _, err := c.ListInbox(context.Background(), provider.ListQuery{OnlyNew: true}); !errors.Is(err, provider.ErrInvalidRequest) {
		t.Fatalf("OnlyNew: %v", err)
	}
}

func TestFetchEInvoice(t *testing.T) {
	pdf := []byte("%PDF-1.4")
	f, srv := newServer(t, map[string]string{
		"getUBL":         `<getUBLResponse xmlns="http:/fitcons.com/eInvoice/"><DocData>` + zipB64(t, "x.xml", []byte(invoiceXML)) + `</DocData></getUBLResponse>`,
		"getInvoiceView": `<getInvoiceViewResponse xmlns="http:/fitcons.com/eInvoice/"><DocData>` + base64.StdEncoding.EncodeToString(pdf) + `</DocData></getInvoiceViewResponse>`,
	})
	c := newClient(t, srv.URL)
	got, err := c.Fetch(context.Background(), provider.FetchQuery{Kind: provider.EInvoice, Direction: provider.Inbound, UUID: "u1"})
	if err != nil || string(got.Data) != invoiceXML {
		t.Fatalf("ubl %v %v", got, err)
	}
	if r := f.last("getUBL"); r.get("Type") != "INBOUND" || r.get("Parameters") != "ZIP" || r.get("Identifier") != "urn:mail:defaultpk@fitcons.com" {
		t.Fatalf("getUBL %+v", r.Fields)
	}
	got, err = c.Fetch(context.Background(), provider.FetchQuery{Kind: provider.EInvoice, Direction: provider.Outbound, UUID: "u2", Format: provider.FormatPDF})
	if err != nil || !bytes.Equal(got.Data, pdf) || got.ContentType != "application/pdf" {
		t.Fatalf("pdf %v %v", got, err)
	}
	if r := f.last("getInvoiceView"); r.get("DocType") != "PDF" || r.get("Type") != "OUTBOUND" || r.get("UUID") != "u2" {
		t.Fatalf("view %+v", r.Fields)
	}
}

func TestEArchive(t *testing.T) {
	signed := base64.StdEncoding.EncodeToString([]byte(strings.Replace(invoiceXML, "TICARIFATURA", "EARSIVFATURA", 1)))
	f, srv := newServer(t, map[string]string{
		"sendInvoice": `<sendInvoiceResponseType xmlns="http://fitcons.com/earchive/invoice"><Detail xmlns="">130:Invoice is signed successfuly</Detail>
<Result xmlns=""><Result>SUCCESS</Result></Result><preCheckErrorResults xmlns=""/><preCheckSuccessResults xmlns=""><preCheckSuccess>
<UUID>F47AC10B-58CC-4372-A567-0E02B2C3D479</UUID><Vkn>11111111199</Vkn><InvoiceNumber>YN12016000000107</InvoiceNumber><SuccessCode>200</SuccessCode>
<SuccessDesc>OK</SuccessDesc><Filename>1invoice.zip</Filename></preCheckSuccess></preCheckSuccessResults></sendInvoiceResponseType>`,
		"getStatus": `<getStatusResponseType xmlns="http://fitcons.com/earchive/invoice"><UUID xmlns="">F47AC10B-58CC-4372-A567-0E02B2C3D479</UUID>
<invoiceNumber xmlns="">YN02015000000024</invoiceNumber><statusCode xmlns="">460</statusCode><Detail xmlns="">iptal raporlandı</Detail></getStatusResponseType>`,
		"getSignedInvoice": `<getSignedInvoiceResponseType xmlns="http://fitcons.com/earchive/invoice"><UUID xmlns="">F47AC10B</UUID><binaryData xmlns="">` + signed + `</binaryData></getSignedInvoiceResponseType>`,
		"cancelInvoice": `<invoiceCancellationServiceResponseType xmlns="http://fitcons.com/earchive/invoicecancellation"><Result xmlns=""><Result>SUCCESS</Result></Result>
<invoiceCancellation xmlns=""><message>Fatura iptal bilgileri basari ile kaydedilmistir.</message><code>1</code></invoiceCancellation></invoiceCancellationServiceResponseType>`,
	})
	c := newClient(t, srv.URL)
	doc := &provider.Document{XML: []byte(strings.Replace(invoiceXML, "TICARIFATURA", "EARSIVFATURA", 1))}
	res, err := c.Send(context.Background(), doc)
	if err != nil {
		t.Fatal(err)
	}
	if res.ID != "YN12016000000107" || res.Status != provider.StatusDelivered || res.ProviderCode != "130" {
		t.Fatalf("send %+v", res)
	}
	r := f.last("sendInvoice")
	if r.NS != nsArc || len(r.ChildNS) != 1 || !r.ChildNS[""] {
		t.Fatalf("namespaces %q %v", r.NS, r.ChildNS)
	}
	if r.get("senderID") != "1234567801" || r.get("receiverID") != "2222222222" || r.get("docType") != "XML" || r.get("fileName") != "F47AC10B-58CC-4372-A567-0E02B2C3D479.zip" {
		t.Fatalf("fields %+v", r.Fields)
	}
	zipped, _ := base64.StdEncoding.DecodeString(r.get("binaryData"))
	if h := r.get("hash"); len(h) != 32 || h != strings.ToUpper(h) || len(zipped) == 0 {
		t.Fatalf("hash %q", h)
	}
	st, err := c.Status(context.Background(), provider.StatusQuery{Kind: provider.EArchive, UUID: doc.UUID})
	if err != nil || st.Status != provider.StatusCancelled || st.ProviderCode != "460" {
		t.Fatalf("status %+v %v", st, err)
	}
	if err := c.CancelEArchive(context.Background(), doc.UUID, "yanlış tutar"); err != nil {
		t.Fatal(err)
	}
	cr := f.last("cancelInvoice")
	if cr.NS != nsCancel || cr.get("invoiceId") != "YN02015000000024" || cr.get("totalAmount") != "206.19" || cr.get("vkn") != "1234567801" || len(cr.get("cancelDate")) != 10 {
		t.Fatalf("cancel %+v", cr.Fields)
	}
}

func TestEArchiveSendError(t *testing.T) {
	_, srv := newServer(t, map[string]string{
		"sendInvoice": `<sendInvoiceResponseType xmlns="http://fitcons.com/earchive/invoice"><Detail xmlns="">FAIL</Detail><Result xmlns=""><Result>FAIL</Result></Result>
<preCheckErrorResults xmlns=""><preCheckError><UUID>u</UUID><ErrorCode>30</ErrorCode><ErrorDesc>VKN/TCKN e-Fatura mükellefine ait.</ErrorDesc></preCheckError></preCheckErrorResults></sendInvoiceResponseType>`,
	})
	_, err := newClient(t, srv.URL).Send(context.Background(), &provider.Document{XML: []byte(strings.Replace(invoiceXML, "TICARIFATURA", "EARSIVFATURA", 1))})
	var pe *provider.Error
	if !errors.As(err, &pe) || pe.Kind != provider.KindRecipient || pe.Code != "30" {
		t.Fatalf("err %v", err)
	}
}
