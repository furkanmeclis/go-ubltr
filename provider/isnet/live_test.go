//go:build live

package isnet_test

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/furkanmeclis/go-ubltr/provider"
	"github.com/furkanmeclis/go-ubltr/provider/isnet"
)

// Live tests call İşNet from this machine's IP, which İşNet must have
// allowlisted for ISNET_VKN. They only read: GetTaxPayer, one-page
// SearchAllInvoice / SearchArchiveInvoice listings and, when a UUID is
// given, status and UBL lookups.
//
//	ISNET_VKN                  company VKN sent as CompanyTaxCode (required)
//	ISNET_LIVE=1               use the live system (default: test system)
//	ISNET_ENDPOINT             InvoiceService URL override
//	ISNET_ADDRESSBOOK_ENDPOINT AddressBookService URL override
//	ISNET_CHECK_VKN            VKN for CheckUser (default ISNET_VKN)
//	ISNET_DAYS                 listing window in days (default 7)
//	ISNET_STATUS_UUID          outgoing e-Fatura ETTN for Status and Fetch
//	ISNET_EARCHIVE_UUID        e-Arşiv ETTN for Status
//
//	ISNET_VKN=... go test -tags live -run Live ./...
func liveClient(t *testing.T) *isnet.Client {
	t.Helper()
	vkn := os.Getenv("ISNET_VKN")
	if vkn == "" {
		t.Skip("ISNET_VKN is not set")
	}
	c, err := isnet.New(isnet.Config{
		VKN:                 vkn,
		Test:                os.Getenv("ISNET_LIVE") != "1",
		Endpoint:            os.Getenv("ISNET_ENDPOINT"),
		AddressBookEndpoint: os.Getenv("ISNET_ADDRESSBOOK_ENDPOINT"),
		Timeout:             45 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func liveFail(t *testing.T, what string, err error) {
	t.Helper()
	if isnet.IsIPNotAllowed(err) {
		t.Fatalf("%s: this machine's IP is probably not allowlisted by İşNet: %v", what, err)
	}
	t.Fatalf("%s: %v", what, err)
}

func TestLiveReadOnly(t *testing.T) {
	c := liveClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	vkn := os.Getenv("ISNET_CHECK_VKN")
	if vkn == "" {
		vkn = os.Getenv("ISNET_VKN")
	}
	u, err := c.CheckUser(ctx, vkn)
	if err != nil {
		liveFail(t, "CheckUser", err)
	}
	t.Logf("CheckUser: %+v", u)

	days := 7
	if s := os.Getenv("ISNET_DAYS"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			days = n
		}
	}
	to := time.Now()
	q := provider.ListQuery{From: to.AddDate(0, 0, -days), To: to, Limit: 5}
	for _, l := range []struct {
		name string
		fn   func(context.Context, provider.ListQuery) (*provider.Page, error)
		kind provider.DocumentKind
	}{
		{"ListInbox", c.ListInbox, provider.EInvoice},
		{"ListOutbox e-Fatura", c.ListOutbox, provider.EInvoice},
		{"ListOutbox e-Arşiv", c.ListOutbox, provider.EArchive},
	} {
		lq := q
		lq.Kind = l.kind
		page, err := l.fn(ctx, lq)
		if err != nil {
			liveFail(t, l.name, err)
		}
		t.Logf("%s: %d items, next %q", l.name, len(page.Items), page.Next)
		for _, d := range page.Items {
			t.Logf("  %s %s %s %s", d.UUID, d.ID, d.Status, d.ProviderCode)
		}
	}
	if id := os.Getenv("ISNET_STATUS_UUID"); id != "" {
		st, err := c.Status(ctx, provider.StatusQuery{Kind: provider.EInvoice, UUID: id})
		if err != nil {
			liveFail(t, "Status", err)
		}
		t.Logf("Status: %s gib %d %s %s", st.Status, st.GIBCode, st.ProviderCode, st.Message)
		fr, err := c.Fetch(ctx, provider.FetchQuery{Kind: provider.EInvoice, Direction: provider.Outbound, UUID: id})
		if err != nil {
			liveFail(t, "Fetch", err)
		}
		t.Logf("Fetch: %d bytes %s", len(fr.Data), fr.ContentType)
	}
	if id := os.Getenv("ISNET_EARCHIVE_UUID"); id != "" {
		st, err := c.Status(ctx, provider.StatusQuery{Kind: provider.EArchive, UUID: id})
		if err != nil {
			liveFail(t, "e-Arşiv Status", err)
		}
		t.Logf("e-Arşiv Status: %s %s %s", st.Status, st.ProviderCode, st.Message)
	}
}
