package provider_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/go-ubltr/provider"
	"github.com/furkanmeclis/go-ubltr/provider/providertest"
)

const sampleInvoice = `<?xml version="1.0" encoding="UTF-8"?>
<Invoice xmlns="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2"
 xmlns:cac="urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2"
 xmlns:cbc="urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2">
 <cbc:ProfileID>TICARIFATURA</cbc:ProfileID>
 <cbc:ID>ABC2026000000001</cbc:ID>
 <cbc:UUID>3f2b1d0e-1111-4222-8333-944455556666</cbc:UUID>
 <cbc:InvoiceTypeCode>SATIS</cbc:InvoiceTypeCode>
 <cac:AccountingSupplierParty><cac:Party><cac:PartyIdentification><cbc:ID schemeID="VKN">1111111111</cbc:ID></cac:PartyIdentification></cac:Party></cac:AccountingSupplierParty>
 <cac:AccountingCustomerParty><cac:Party><cac:PartyIdentification><cbc:ID schemeID="MERSISNO">0</cbc:ID></cac:PartyIdentification><cac:PartyIdentification><cbc:ID schemeID="VKN">2222222222</cbc:ID></cac:PartyIdentification></cac:Party></cac:AccountingCustomerParty>
 <cac:InvoiceLine><cbc:ID>1</cbc:ID></cac:InvoiceLine>
</Invoice>`

func TestDocumentInspect(t *testing.T) {
	d := &provider.Document{XML: []byte(sampleInvoice)}
	if err := d.Inspect(); err != nil {
		t.Fatal(err)
	}
	if d.Kind != provider.EInvoice || d.UUID != "3f2b1d0e-1111-4222-8333-944455556666" || d.ID != "ABC2026000000001" ||
		d.Profile != "TICARIFATURA" || d.Type != "SATIS" || d.ReceiverVKN != "2222222222" {
		t.Fatalf("got %+v", d)
	}
	ea := &provider.Document{XML: []byte(strings.Replace(sampleInvoice, "TICARIFATURA", "EARSIVFATURA", 1))}
	if err := ea.Inspect(); err != nil || ea.Kind != provider.EArchive {
		t.Fatalf("earchive kind = %q, %v", ea.Kind, err)
	}
	if err := (&provider.Document{XML: []byte("<Foo/>")}).Inspect(); err == nil {
		t.Fatal("expected error for unknown root")
	}
}

func TestErrorIsAndKind(t *testing.T) {
	err := error(&provider.Error{Kind: provider.KindQuotaExceeded, Provider: "x", Code: "E12", Message: "kontör yok"})
	wrapped := errors.Join(errors.New("ctx"), err)
	if !errors.Is(wrapped, provider.ErrQuotaExceeded) {
		t.Fatal("errors.Is sentinel failed")
	}
	if errors.Is(wrapped, provider.ErrAuth) {
		t.Fatal("matched wrong sentinel")
	}
	if provider.KindOf(wrapped) != provider.KindQuotaExceeded {
		t.Fatal("KindOf")
	}
	if !strings.Contains(err.Error(), "E12") || !strings.Contains(err.Error(), "kontör yok") {
		t.Fatal(err.Error())
	}
	if provider.IsRetryable(err) || !provider.IsRetryable(&provider.Error{Kind: provider.KindRateLimit}) {
		t.Fatal("IsRetryable")
	}
}

func TestGIBCodes(t *testing.T) {
	providertest.CheckCodeTable(t, provider.GIBCodes)
	if provider.GIBStatus(1300) != provider.StatusDelivered || provider.GIBStatus(1000) != provider.StatusQueued {
		t.Fatal("GIBStatus")
	}
	if provider.GIBError("x", "status", 1200, "") != nil {
		t.Fatal("1200 is not an error")
	}
	e := provider.GIBError("x", "status", 1163, "")
	if !errors.Is(e, provider.ErrDuplicate) || e.GIBCode != 1163 {
		t.Fatalf("1163 => %v", e)
	}
	if !errors.Is(provider.GIBError("x", "status", 1230, ""), provider.ErrRecipientResponse) {
		t.Fatal("1230")
	}
	if !provider.GIBError("x", "status", 1195, "").Retryable {
		t.Fatal("1195 retryable")
	}
}

func TestParseCodeTable(t *testing.T) {
	tb, err := provider.ParseCodeTable("v", strings.NewReader("# c\ncode,kind,status,retryable,message,message_en\nE1,auth,,false,Hatalı şifre,Bad password\nE2,rate-limit,,true,Çok istek\n"))
	if err != nil {
		t.Fatal(err)
	}
	e := tb.NewError("v", "send", "E1", "")
	if !errors.Is(e, provider.ErrAuth) || e.Message != "Hatalı şifre" {
		t.Fatalf("%v", e)
	}
	if !tb.NewError("v", "send", "E2", "").Retryable {
		t.Fatal("retryable")
	}
	u := tb.NewError("v", "send", "ZZZ", "raw msg")
	if u.Kind != provider.KindUnknown || u.Code != "ZZZ" || u.Message != "raw msg" {
		t.Fatalf("unknown: %+v", u)
	}
	g := tb.NewError("v", "send", "1160", "")
	if !errors.Is(g, provider.ErrValidation) || g.GIBCode != 1160 {
		t.Fatalf("gib fallback: %+v", g)
	}
	for _, bad := range []string{
		"x,y\n",
		"code,kind,status,retryable,message\nE1,nope,,false,m\n",
		"code,kind,status,retryable,message\nE1,auth,weird,false,m\n",
		"code,kind,status,retryable,message\nE1,auth,,maybe,m\n",
		"code,kind,status,retryable,message\nE1,auth,,false,m\nE1,auth,,false,m\n",
	} {
		if _, err := provider.ParseCodeTable("bad", strings.NewReader(bad)); err == nil {
			t.Errorf("want error for %q", bad)
		}
	}
}

func TestRetry(t *testing.T) {
	n := 0
	err := provider.Retry(context.Background(), provider.RetryPolicy{Attempts: 4, Base: time.Millisecond}, func(context.Context) error {
		n++
		if n < 3 {
			return &provider.Error{Kind: provider.KindUnavailable}
		}
		return nil
	})
	if err != nil || n != 3 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	n = 0
	err = provider.Retry(context.Background(), provider.RetryPolicy{Attempts: 4, Base: time.Millisecond}, func(context.Context) error {
		n++
		return &provider.Error{Kind: provider.KindValidation}
	})
	if n != 1 || !errors.Is(err, provider.ErrValidation) {
		t.Fatalf("non-retryable: n=%d", n)
	}
}

func TestHTTPError(t *testing.T) {
	cases := map[int]provider.ErrorKind{401: provider.KindAuth, 403: provider.KindPermission, 404: provider.KindNotFound, 429: provider.KindRateLimit, 503: provider.KindUnavailable, 400: provider.KindInvalidRequest}
	for code, want := range cases {
		if got := provider.HTTPError("x", "op", code, nil).Kind; got != want {
			t.Errorf("%d: %s want %s", code, got, want)
		}
	}
}

func TestFakeConformance(t *testing.T) {
	f := providertest.NewFake()
	providertest.CheckCapabilities(t, f)
	ctx := context.Background()
	res, err := f.Send(ctx, &provider.Document{XML: []byte(sampleInvoice)})
	if err != nil || res.Status != provider.StatusQueued {
		t.Fatalf("send: %v %+v", err, res)
	}
	if _, err := f.Send(ctx, &provider.Document{XML: []byte(sampleInvoice)}); !errors.Is(err, provider.ErrDuplicate) {
		t.Fatalf("duplicate: %v", err)
	}
	for i := range 5 {
		f.Deliver(provider.DocumentInfo{Kind: provider.EInvoice, UUID: string(rune('a' + i)), CreatedAt: time.Unix(int64(i), 0)}, []byte("<Invoice/>"))
	}
	p1, _ := f.ListInbox(ctx, provider.ListQuery{Limit: 2, OnlyNew: true})
	p2, _ := f.ListInbox(ctx, provider.ListQuery{Limit: 2, OnlyNew: true, Cursor: p1.Next})
	if len(p1.Items) != 2 || p1.Items[0].UUID != "a" || p2.Items[0].UUID != "c" {
		t.Fatalf("paging: %+v %+v", p1, p2)
	}
	_ = f.MarkTaken(ctx, provider.Inbound, "a", "b")
	p, _ := f.ListInbox(ctx, provider.ListQuery{OnlyNew: true})
	if len(p.Items) != 3 {
		t.Fatalf("after MarkTaken: %d", len(p.Items))
	}
	if err := f.Respond(ctx, provider.Response{UUID: "c", Accept: true}); err != nil {
		t.Fatal(err)
	}
	if err := f.Respond(ctx, provider.Response{UUID: "c"}); !errors.Is(err, provider.ErrInvalidState) {
		t.Fatalf("second respond: %v", err)
	}
}

func TestRegistry(t *testing.T) {
	provider.Register("test-fake", func(provider.Config) (provider.Provider, error) { return providertest.NewFake(), nil })
	p, err := provider.Open("test-fake", provider.Config{})
	if err != nil || p.Name() != "fake" {
		t.Fatal(err)
	}
	if _, err := provider.Open("nope", provider.Config{}); err == nil {
		t.Fatal("want error")
	}
}
