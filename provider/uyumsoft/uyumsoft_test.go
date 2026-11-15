package uyumsoft

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/go-ubltr/provider"
	"github.com/furkanmeclis/go-ubltr/provider/providertest"
)

var ctx = context.Background()

func TestCapabilitiesAndCodes(t *testing.T) {
	for _, p := range []Preset{Uyumsoft, Muhasebeizleme} {
		c, err := New(Config{Username: "u", Password: "p", Preset: p})
		if err != nil {
			t.Fatal(err)
		}
		if c.Name() != p.Name {
			t.Errorf("Name = %q", c.Name())
		}
		providertest.CheckCapabilities(t, c)
		if c.Capabilities().Has(provider.EDespatch, provider.OpSend) {
			t.Errorf("%s: claims e-İrsaliye", p.Name)
		}
		// Capabilities must be a copy.
		c.Capabilities().Operations[provider.EInvoice][0] = "x"
		if c.Capabilities().Operations[provider.EInvoice][0] == "x" {
			t.Error("Capabilities shares state")
		}
	}
	providertest.CheckCodeTable(t, Codes)
}

func TestRegistry(t *testing.T) {
	for _, name := range []string{"uyumsoft", "muhasebeizleme"} {
		p, err := provider.Open(name, provider.Config{Username: "u", APIKey: "k", Test: true, Options: map[string]string{"scenario": "Automated", "timestamp": "false"}})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		c := p.(*Client)
		if c.Name() != name || c.cfg.Scenario != ScenarioAutomated || !c.cfg.NoTimestamp || c.cfg.Password != "k" {
			t.Errorf("%s: config not applied: %+v", name, c.cfg)
		}
		if !strings.Contains(c.soap.URL, "test") {
			t.Errorf("%s: test URL = %s", name, c.soap.URL)
		}
	}
	if _, err := provider.Open("uyumsoft", provider.Config{Username: "u", Password: "p", Options: map[string]string{"scenario": "x"}}); err == nil {
		t.Error("bad scenario accepted")
	}
	if _, err := New(Config{Password: "p"}); err == nil {
		t.Error("missing username accepted for WS-Security preset")
	}
	if _, err := New(Config{Preset: Muhasebeizleme, Password: "k"}); err != nil {
		t.Errorf("muhasebeizleme without username: %v", err)
	}
}

func TestEndpoints(t *testing.T) {
	cases := []struct {
		p    Preset
		test bool
		want string
	}{
		{Uyumsoft, true, "https://efatura-test.uyumsoft.com.tr/Services/Integration"},
		{Uyumsoft, false, "https://efatura.uyumsoft.com.tr/Services/Integration"},
		{Muhasebeizleme, true, "https://test.api.muhasebeizleme.com/Services/BasicIntegration.svc"},
		{Muhasebeizleme, false, "https://api.muhasebeizleme.com/Services/BasicIntegration.svc"},
	}
	for _, tc := range cases {
		c, _ := New(Config{Username: "u", Password: "p", Preset: tc.p, Test: tc.test})
		if c.soap.URL != tc.want {
			t.Errorf("%s test=%v: %s", tc.p.Name, tc.test, c.soap.URL)
		}
	}
	c, _ := New(Config{Username: "u", Password: "p", Endpoint: "https://example.invalid/x"})
	if c.soap.URL != "https://example.invalid/x" {
		t.Error("Endpoint override ignored")
	}
}

func TestSendUyumsoft(t *testing.T) {
	c, f := uyumsoftClient(t)
	doc := &provider.Document{XML: readFile(t, "invoice.xml"), ReceiverAlias: "urn:mail:defaultpk@ornek.com.tr"}
	res, err := c.Send(ctx, doc)
	if err != nil {
		t.Fatal(err)
	}
	if res.UUID != "3f2b8c1e-7a4d-4e8b-9c0f-12ab34cd56ef" || res.ID != "ABC2026000000123" || res.Status != provider.StatusQueued || res.ProviderCode != "eInvoice" {
		t.Errorf("result = %+v", res)
	}
	body := f.body("SendInvoice")
	mustContain(t, body,
		`<Security xmlns="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-secext-1.0.xsd">`,
		`>user</Username>`, `#PasswordText">pass</Password>`,
		`>2026-11-15T09:00:00.000Z</Created>`, `>2026-11-15T09:05:00.000Z</Expires>`,
		`<SendInvoice xmlns="http://tempuri.org/"><invoices><InvoiceInfo><Invoice xmlns:cac=`,
		`<cbc:ProfileID>TEMELFATURA</cbc:ProfileID>`,
		`<TargetCustomer VknTckn="9876543210" Alias="urn:mail:defaultpk@ornek.com.tr"></TargetCustomer>`,
		`<Scenario>eInvoice</Scenario>`, `<CreateDateUtc>2026-11-15T12:00:00</CreateDateUtc>`)
	if strings.Contains(body, "userInfo") || strings.Contains(body, "schemaLocation") {
		t.Error("unexpected userInfo or schemaLocation in Uyumsoft request")
	}
	if got := f.actions[0]; got != "http://tempuri.org/IIntegration/SendInvoice" {
		t.Errorf("SOAPAction = %s", got)
	}
	checkEmbeddedInvoice(t, body)
}

// checkEmbeddedInvoice parses the request and verifies InvoiceInfo/Invoice
// is in the tempuri namespace with UBL cbc children.
func checkEmbeddedInvoice(t *testing.T, body string) {
	t.Helper()
	dec := xml.NewDecoder(strings.NewReader(body))
	var inInvoice bool
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		if se.Name.Local == "Invoice" {
			if se.Name.Space != nsTempuri {
				t.Errorf("Invoice namespace = %q", se.Name.Space)
			}
			inInvoice = true
			continue
		}
		if inInvoice && se.Name.Local == "UUID" {
			if se.Name.Space != "urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2" {
				t.Errorf("cbc:UUID namespace = %q", se.Name.Space)
			}
			return
		}
	}
	t.Error("embedded cbc:UUID not found")
}

func TestSendEArchiveOptions(t *testing.T) {
	c, f := uyumsoftClient(t)
	ubl := bytes.Replace(readFile(t, "invoice.xml"), []byte("TEMELFATURA"), []byte("EARSIVFATURA"), 1)
	_, err := c.SendInvoices(ctx, Invoice{
		Document:        &provider.Document{XML: ubl, Email: "alici@ornek.com.tr"},
		LocalDocumentID: "ERP-1",
		Mail:            []Mail{{To: "muhasebe@ornek.com.tr", Subject: "Fatura", AttachPDF: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	mustContain(t, f.body("SendInvoice"),
		`<InvoiceInfo LocalDocumentId="ERP-1">`,
		`<EArchiveInvoiceInfo DeliveryType="Electronic"></EArchiveInvoiceInfo><Scenario>eArchive</Scenario>`,
		`<Mailing EnableNotification="true" To="alici@ornek.com.tr"></Mailing>`,
		`<Mailing EnableNotification="true" To="muhasebe@ornek.com.tr"><Subject>Fatura</Subject><Attachment Xml="false" Pdf="true" Html="false" AdditionalDocuments="false"></Attachment></Mailing>`)
}

func TestSendFailures(t *testing.T) {
	c, f := uyumsoftClient(t)
	f.override["SendInvoice"] = "SendInvoice_failed"
	_, err := c.Send(ctx, &provider.Document{XML: readFile(t, "invoice.xml")})
	if !errors.Is(err, provider.ErrValidation) {
		t.Fatalf("IsSucceded=false: got %v", err)
	}
	var pe *provider.Error
	if !errors.As(err, &pe) || pe.Message == "" || pe.Code != "" || len(pe.Raw) == 0 {
		t.Errorf("error = %+v", pe)
	}

	f.override["SendInvoice"] = "fault_auth"
	f.status["SendInvoice"] = http.StatusInternalServerError
	_, err = c.Send(ctx, &provider.Document{XML: readFile(t, "invoice.xml")})
	if !errors.Is(err, provider.ErrAuth) || provider.IsRetryable(err) {
		t.Fatalf("WS-Security fault: got %v", err)
	}

	if _, err := c.Send(ctx, &provider.Document{XML: []byte(`<DespatchAdvice/>`)}); !errors.Is(err, provider.ErrValidation) {
		t.Errorf("despatch: %v", err)
	}
	if _, err := c.Send(ctx, &provider.Document{}); !errors.Is(err, provider.ErrValidation) {
		t.Errorf("empty: %v", err)
	}
}

func TestSendMuhasebeizleme(t *testing.T) {
	c, f := miClient(t)
	ubl := readFile(t, "invoice.xml")
	c.cfg.Scenario = ScenarioAutomated
	out, err := c.SendInvoices(ctx,
		Invoice{Document: &provider.Document{XML: ubl}, LocalDocumentID: "ERP-1"},
		Invoice{Document: &provider.Document{XML: ubl}, LocalDocumentID: "ERP-2"})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 || out[0].Err != nil || out[0].Result.UUID != "3f2b8c1e-7a4d-4e8b-9c0f-12ab34cd56ef" || out[0].Result.ProviderCode != "eArchive" {
		t.Fatalf("out[0] = %+v", out[0])
	}
	if !errors.Is(out[1].Err, provider.ErrQuotaExceeded) {
		t.Errorf("out[1] = %v", out[1].Err)
	}
	body := f.body("SendInvoice")
	mustContain(t, body, `<SendInvoice xmlns="http://tempuri.org/"><userInfo Username="entegrasyon" Password="api-key"></userInfo><invoices>`,
		`<Scenario>Automated</Scenario>`)
	if strings.Contains(body, "Security") {
		t.Error("WS-Security header sent to BasicIntegration")
	}
	if f.actions[0] != "http://tempuri.org/IBasicIntegration/SendInvoice" {
		t.Errorf("SOAPAction = %s", f.actions[0])
	}
}

func TestStatusMapping(t *testing.T) {
	out := provider.Outbound
	in := provider.Inbound
	cases := []struct {
		p    Preset
		in   statusInput
		want provider.Status
		resp string
	}{
		{Uyumsoft, statusInput{Raw: "NotPrepared", Direction: out}, provider.StatusDraft, ""},
		{Uyumsoft, statusInput{Raw: "NotSend", Direction: out}, provider.StatusDraft, ""},
		{Uyumsoft, statusInput{Raw: "Draft", Direction: out}, provider.StatusDraft, ""},
		{Uyumsoft, statusInput{Raw: "Canceled", Direction: out}, provider.StatusCancelled, ""},
		{Uyumsoft, statusInput{Raw: "Queued", Direction: out}, provider.StatusQueued, ""},
		{Uyumsoft, statusInput{Raw: "Processing", Direction: out}, provider.StatusProcessing, ""},
		{Uyumsoft, statusInput{Raw: "SentToGib", Direction: out}, provider.StatusSent, ""},
		{Uyumsoft, statusInput{Raw: "Approved", Direction: out}, provider.StatusDelivered, ""},
		{Uyumsoft, statusInput{Raw: "Approved", Direction: out, Commercial: true}, provider.StatusAccepted, "KABUL"},
		{Uyumsoft, statusInput{Raw: "WaitingForAprovement", Direction: out, Commercial: true}, provider.StatusDelivered, ""},
		{Uyumsoft, statusInput{Raw: "Declined", Direction: out, Commercial: true}, provider.StatusRejected, "RED"},
		{Uyumsoft, statusInput{Raw: "Return", Direction: out, Commercial: true}, provider.StatusRejected, "IADE"},
		{Uyumsoft, statusInput{Raw: "EArchivedCanceled", Direction: out}, provider.StatusCancelled, ""},
		{Uyumsoft, statusInput{Raw: "Error", Direction: out}, provider.StatusFailed, ""},
		{Uyumsoft, statusInput{Raw: "Approved", Direction: in}, provider.StatusReceived, ""},
		{Uyumsoft, statusInput{Raw: "Approved", Direction: in, Commercial: true}, provider.StatusAccepted, "KABUL"},
		{Uyumsoft, statusInput{Raw: "WaitingForAprovement", Direction: in}, provider.StatusReceived, ""},
		{Uyumsoft, statusInput{Raw: "Declined", Direction: in}, provider.StatusRejected, "RED"},
		{Uyumsoft, statusInput{Raw: "Declined", Direction: out}, provider.StatusRejected, ""},
		{Uyumsoft, statusInput{Raw: "Bogus", Code: 1000, Direction: out}, provider.StatusUnknown, ""},
		{Muhasebeizleme, statusInput{Raw: "Bogus", Code: 1000, Direction: out}, provider.StatusDelivered, ""},
		{Muhasebeizleme, statusInput{Code: 10, Direction: out}, provider.StatusCancelled, ""},
		{Muhasebeizleme, statusInput{Code: 1400, Direction: out}, provider.StatusCancelled, ""},
		{Muhasebeizleme, statusInput{Code: 1200, Direction: out}, provider.StatusRejected, ""},
		{Muhasebeizleme, statusInput{Code: 2000, Direction: out}, provider.StatusFailed, ""},
		{Muhasebeizleme, statusInput{Code: 300, Direction: out}, provider.StatusSent, ""},
		{Muhasebeizleme, statusInput{Code: 777, Direction: out}, provider.StatusUnknown, ""},
	}
	for _, tc := range cases {
		if got := mapStatus(tc.p, tc.in); got != tc.want {
			t.Errorf("%s %+v: %s, want %s", tc.p.Name, tc.in, got, tc.want)
		}
		if got := responseCode(tc.in); got != tc.resp {
			t.Errorf("%+v: response %q, want %q", tc.in, got, tc.resp)
		}
	}
}

// Every InvoiceStatus enum value in the WSDL has a row in errors.csv.
func TestStatusEnumCovered(t *testing.T) {
	for _, v := range wsdlEnum(t, "testdata/Integration.wsdl", "InvoiceStatus") {
		if _, ok := Codes.Lookup("status:" + v); !ok {
			t.Errorf("InvoiceStatus %s has no errors.csv row", v)
		}
	}
	for _, v := range wsdlEnum(t, "testdata/Integration.wsdl", "EnvelopeStatus") {
		switch v {
		case "NoEnvelope", "Preparing", "DoesNotHaveEntegratorApplication":
			continue
		}
		n, ok := envelopeCodes[v]
		if !ok {
			t.Errorf("EnvelopeStatus %s has no GİB code", v)
			continue
		}
		if n < 2000 && provider.GIBStatus(n) == provider.StatusUnknown && n != 1164 && n != 1235 {
			t.Errorf("EnvelopeStatus %s → %d not in GİB table", v, n)
		}
	}
	for _, v := range wsdlEnum(t, "testdata/Integration.wsdl", "InvoiceTypes") {
		if profiles[v] == "" {
			t.Errorf("InvoiceTypes %s unmapped", v)
		}
	}
	for _, v := range wsdlEnum(t, "testdata/Integration.wsdl", "InvoiceTipType") {
		if invoiceTypes[v] == "" {
			t.Errorf("InvoiceTipType %s unmapped", v)
		}
	}
}

func TestStatusQueries(t *testing.T) {
	c, f := uyumsoftClient(t)
	r, err := c.Status(ctx, provider.StatusQuery{UUID: "3f2b8c1e-7a4d-4e8b-9c0f-12ab34cd56ef"})
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != provider.StatusSent || r.ProviderCode != "SentToGib" {
		t.Errorf("outbox = %+v", r)
	}
	mustContain(t, f.body("QueryOutboxInvoiceStatus"), `<QueryOutboxInvoiceStatus xmlns="http://tempuri.org/"><invoiceIds><string>3f2b8c1e-7a4d-4e8b-9c0f-12ab34cd56ef</string></invoiceIds>`)
	r, err = c.Status(ctx, provider.StatusQuery{Direction: provider.Inbound, UUID: "b7c8d9e0-1a2b-4c3d-8e9f-a0b1c2d3e4f5"})
	if err != nil || r.Status != provider.StatusReceived {
		t.Errorf("inbox = %+v, %v", r, err)
	}
	if _, err := c.Status(ctx, provider.StatusQuery{Direction: provider.Inbound, Kind: provider.EArchive, UUID: "x"}); !errors.Is(err, provider.ErrUnsupported) {
		t.Errorf("inbound e-Arşiv: %v", err)
	}

	m, mf := miClient(t)
	r, err = m.Status(ctx, provider.StatusQuery{UUID: "3f2b8c1e-7a4d-4e8b-9c0f-12ab34cd56ef"})
	if err != nil || r.Status != provider.StatusDelivered {
		t.Errorf("mi = %+v, %v", r, err)
	}
	mf.override["QueryOutboxInvoiceStatus"] = "QueryOutboxInvoiceStatus_unknown"
	if _, err := m.Status(ctx, provider.StatusQuery{UUID: "00000000-0000-0000-0000-000000000000"}); !errors.Is(err, provider.ErrNotFound) {
		t.Errorf("unknown ETTN: %v", err)
	}
}

func TestListInbox(t *testing.T) {
	c, f := uyumsoftClient(t)
	from := time.Date(2026, 9, 1, 0, 0, 0, 0, trt)
	p, err := c.ListInbox(ctx, provider.ListQuery{OnlyNew: true, From: from, Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	mustContain(t, f.body("GetInboxInvoiceList"),
		`<query PageIndex="0" PageSize="2" OnlyNewestInvoices="true"><CreateStartDate>2026-09-01T00:00:00</CreateStartDate><IncludeTagList>false</IncludeTagList></query>`)
	if len(p.Items) != 2 || p.Next != "1" {
		t.Fatalf("page = %+v", p)
	}
	d := p.Items[0]
	if d.UUID != "b7c8d9e0-1a2b-4c3d-8e9f-a0b1c2d3e4f5" || d.ID != "XYZ2026000000045" || d.Profile != "TICARIFATURA" ||
		d.Type != "SATIS" || d.Status != provider.StatusReceived || d.GIBCode != 1300 || d.SenderVKN != "9876543210" ||
		d.Payable != "22003.20" || d.Currency != "TRY" || d.IssueDate != "2026-09-10" || d.Kind != provider.EInvoice || d.Direction != provider.Inbound {
		t.Errorf("item = %+v", d)
	}
	if d.CreatedAt.IsZero() || d.CreatedAt.UTC().Hour() != 13 {
		t.Errorf("CreatedAt = %v", d.CreatedAt)
	}
	if p.Items[1].Status != provider.StatusReceived || p.Items[1].Profile != "TEMELFATURA" {
		t.Errorf("item 2 = %+v", p.Items[1])
	}
	if _, err := c.ListInbox(ctx, provider.ListQuery{Cursor: "x"}); !errors.Is(err, provider.ErrInvalidRequest) {
		t.Errorf("bad cursor: %v", err)
	}
	if _, err := c.ListInbox(ctx, provider.ListQuery{Cursor: "1"}); err != nil {
		t.Fatal(err)
	}
	mustContain(t, f.body("GetInboxInvoiceList"), `PageIndex="1" PageSize="100" OnlyNewestInvoices="false"`)

	m, mf := miClient(t)
	p, err = m.ListInbox(ctx, provider.ListQuery{OnlyNew: true, Limit: 5000})
	if err != nil {
		t.Fatal(err)
	}
	mustContain(t, mf.body("GetInboxInvoiceList"), `<userInfo Username="entegrasyon" Password="api-key"></userInfo><query PageIndex="0" PageSize="1000"><IsArchived>false</IsArchived></query>`)
	if len(p.Items) != 1 || p.Next != "" || p.Items[0].Status != provider.StatusAccepted {
		t.Errorf("mi page = %+v", p)
	}
}

func TestListOutbox(t *testing.T) {
	c, f := uyumsoftClient(t)
	p, err := c.ListOutbox(ctx, provider.ListQuery{Kind: provider.EArchive})
	if err != nil {
		t.Fatal(err)
	}
	mustContain(t, f.body("GetOutboxInvoiceList"), `<IncludeTagList>false</IncludeTagList><Scenario>eArchive</Scenario></query>`)
	// The fixture holds one e-Fatura and one e-Arşiv row; Kind filters.
	if len(p.Items) != 1 || p.Next != "" {
		t.Fatalf("page = %+v", p)
	}
	d := p.Items[0]
	if d.Kind != provider.EArchive || d.Status != provider.StatusFailed || d.Type != "IADE" || d.Profile != "EARSIVFATURA" || d.ReceiverVKN != "9876543210" || d.GIBCode != 0 {
		t.Errorf("item = %+v", d)
	}
	p, _ = c.ListOutbox(ctx, provider.ListQuery{})
	if len(p.Items) != 2 || p.Items[0].Status != provider.StatusDelivered || p.Items[0].GIBCode != 1300 {
		t.Errorf("all = %+v", p.Items)
	}
	if _, err := c.ListOutbox(ctx, provider.ListQuery{OnlyNew: true}); !errors.Is(err, provider.ErrInvalidRequest) {
		t.Errorf("OnlyNew on outbox: %v", err)
	}
	if _, err := c.ListInbox(ctx, provider.ListQuery{Kind: provider.EArchive}); !errors.Is(err, provider.ErrUnsupported) {
		t.Errorf("e-Arşiv inbox: %v", err)
	}
}

func TestFetch(t *testing.T) {
	c, f := uyumsoftClient(t)
	r, err := c.Fetch(ctx, provider.FetchQuery{UUID: "3f2b8c1e-7a4d-4e8b-9c0f-12ab34cd56ef"})
	if err != nil {
		t.Fatal(err)
	}
	if r.Format != provider.FormatUBL || !bytes.Contains(r.Data, []byte("<cbc:ID>ABC2026000000123</cbc:ID>")) {
		t.Errorf("ubl = %s", r.Data)
	}
	mustContain(t, f.body("GetOutboxInvoiceData"), `<GetOutboxInvoiceData xmlns="http://tempuri.org/"><invoiceId>3f2b8c1e-7a4d-4e8b-9c0f-12ab34cd56ef</invoiceId>`)
	r, err = c.Fetch(ctx, provider.FetchQuery{Direction: provider.Inbound, UUID: "b7c8", Format: provider.FormatPDF})
	if err != nil || !bytes.HasPrefix(r.Data, []byte("%PDF-")) || r.ContentType != "application/pdf" {
		t.Errorf("pdf = %v %v", r, err)
	}
	r, err = c.Fetch(ctx, provider.FetchQuery{UUID: "x", Format: provider.FormatHTML})
	if err != nil || string(r.Data) != "<html><body>Fatura</body></html>" {
		t.Errorf("html = %v %v", r, err)
	}
	if _, err := c.Fetch(ctx, provider.FetchQuery{UUID: "x", Format: "docx"}); !errors.Is(err, provider.ErrInvalidRequest) {
		t.Errorf("bad format: %v", err)
	}

	m, _ := miClient(t)
	r, err = m.Fetch(ctx, provider.FetchQuery{UUID: "x", Format: provider.FormatHTML})
	if err != nil || !strings.HasPrefix(string(r.Data), "<!DOCTYPE html>") {
		t.Errorf("mi html = %v %v", r, err)
	}
}

func TestRespond(t *testing.T) {
	c, f := uyumsoftClient(t)
	if err := c.Respond(ctx, provider.Response{UUID: "b7c8", Accept: false, Reason: "Hatalı tutar"}); err != nil {
		t.Fatal(err)
	}
	mustContain(t, f.body("SendDocumentResponse"),
		`<responses><DocumentResponseInfo><InvoiceId>b7c8</InvoiceId><ResponseStatus>Declined</ResponseStatus><Reason>Hatalı tutar</Reason></DocumentResponseInfo></responses>`)

	m, _ := miClient(t)
	err := m.Respond(ctx, provider.Response{UUID: "b7c8", Accept: true})
	if !errors.Is(err, provider.ErrInvalidState) {
		t.Errorf("already answered: %v", err)
	}
}

func TestCancelEArchive(t *testing.T) {
	c, f := uyumsoftClient(t)
	if err := c.CancelEArchive(ctx, "3f2b", "ignored"); err != nil {
		t.Fatal(err)
	}
	mustContain(t, f.body("CancelEArchiveInvoice"), `<request InvoiceId="3f2b" CancelDate="2026-11-15T12:00:00"></request>`)
	m, _ := miClient(t)
	if err := m.CancelEArchive(ctx, "3f2b", ""); !errors.Is(err, provider.ErrInvalidState) {
		t.Errorf("Value=false: %v", err)
	}
}

func TestCheckUser(t *testing.T) {
	c, f := uyumsoftClient(t)
	u, err := c.CheckUser(ctx, "9876543210")
	if err != nil {
		t.Fatal(err)
	}
	if !u.Registered || u.Title != "ALICI TİCARET LİMİTED ŞİRKETİ" || len(u.Aliases) != 3 {
		t.Fatalf("user = %+v", u)
	}
	if a := u.Aliases[0]; a.Name != "urn:mail:defaultpk@ornek.com.tr" || a.Type != "PK" || a.Kind != provider.EInvoice || a.CreatedAt.IsZero() {
		t.Errorf("alias 0 = %+v", a)
	}
	if a := u.Aliases[1]; a.Type != "GB" {
		t.Errorf("alias 1 = %+v", a)
	}
	if a := u.Aliases[2]; a.Kind != provider.EDespatch || a.Type != "PK" {
		t.Errorf("alias 2 = %+v", a)
	}
	mustContain(t, f.body("IsEInvoiceUser"), `<IsEInvoiceUser xmlns="http://tempuri.org/"><vknTckn>9876543210</vknTckn></IsEInvoiceUser>`)
	mustContain(t, f.body("GetUserAliasses"), `<GetUserAliasses xmlns="http://tempuri.org/"><vknTckn>9876543210</vknTckn></GetUserAliasses>`)

	m, mf := miClient(t)
	u, err = m.CheckUser(ctx, "11111111111")
	if err != nil || u.Registered || len(u.Aliases) != 0 {
		t.Errorf("mi = %+v, %v", u, err)
	}
	if mf.body("GetUserAliasses") != "" {
		t.Error("aliases queried for a non-user")
	}
}

func TestMarkTaken(t *testing.T) {
	c, f := uyumsoftClient(t)
	if err := c.MarkTaken(ctx, provider.Inbound, "a", "b"); err != nil {
		t.Fatal(err)
	}
	mustContain(t, f.body("SetInvoicesTaken"), `<invoices><string>a</string><string>b</string></invoices>`)
	if err := c.MarkTaken(ctx, provider.Outbound, "a"); !errors.Is(err, provider.ErrUnsupported) {
		t.Errorf("outbound: %v", err)
	}
	m, mf := miClient(t)
	if err := m.MarkTaken(ctx, provider.Inbound, "a"); err != nil {
		t.Fatal(err)
	}
	mustContain(t, mf.body("SetInvoicesTaken"), `<invoiceIds><string>a</string></invoiceIds>`)
}

func TestCredits(t *testing.T) {
	c, f := uyumsoftClient(t)
	ci, err := c.Credits(ctx)
	if err != nil || ci.Remaining != 1500 {
		t.Errorf("credits = %+v, %v", ci, err)
	}
	if f.actions[0] != "http://tempuri.org/ICustomerServiceBase/GetCustomerCreditInfo" {
		t.Errorf("SOAPAction = %s", f.actions[0])
	}
	m, mf := miClient(t)
	ci, err = m.Credits(ctx)
	if err != nil || ci.Remaining != 4850 {
		t.Errorf("mi credits = %+v, %v", ci, err)
	}
	mf.override["GetCustomerCreditInfo"] = "GetCustomerCreditInfo_failed"
	_, err = m.Credits(ctx)
	if !errors.Is(err, provider.ErrUnavailable) || !provider.IsRetryable(err) {
		t.Errorf("transient: %v", err)
	}
}

func TestPingAndFaults(t *testing.T) {
	c, _ := uyumsoftClient(t)
	if err := c.Ping(ctx); err != nil {
		t.Fatal(err)
	}
	m, mf := miClient(t)
	mf.override["TestConnection"] = "fault_apikey"
	mf.status["TestConnection"] = http.StatusInternalServerError
	err := m.Ping(ctx)
	var pe *provider.Error
	if !errors.As(err, &pe) || pe.Kind != provider.KindAuth || pe.Code != "s:Client" || pe.Op != "TestConnection" || pe.Retryable {
		t.Errorf("api key fault: %+v", pe)
	}
}

func TestNoTimestamp(t *testing.T) {
	c, f := uyumsoftClient(t)
	c.cfg.NoTimestamp = true
	if err := c.Ping(ctx); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(f.body("TestConnection"), "Timestamp") {
		t.Error("Timestamp sent with NoTimestamp")
	}
}
