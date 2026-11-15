//go:build live

package sovos_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/furkanmeclis/go-ubltr/provider"
	"github.com/furkanmeclis/go-ubltr/provider/sovos"
)

// Live tests run against the Sovos test system with an account issued by
// Sovos:
//
//	SOVOS_USERNAME, SOVOS_PASSWORD, SOVOS_VKN   web-service user (required)
//	SOVOS_GB, SOVOS_PK                          GB / PK aliases (required)
//	SOVOS_STATUS_UUID                           outgoing e-Fatura ETTN (optional)
//	SOVOS_EARCHIVE_UUID                         e-Arşiv ETTN (optional)
//
//	go test -tags live -run Live ./...
func TestLiveReadOnly(t *testing.T) {
	user, pass, vkn := os.Getenv("SOVOS_USERNAME"), os.Getenv("SOVOS_PASSWORD"), os.Getenv("SOVOS_VKN")
	if user == "" || pass == "" || vkn == "" || os.Getenv("SOVOS_PK") == "" {
		t.Skip("SOVOS_USERNAME, SOVOS_PASSWORD, SOVOS_VKN and SOVOS_PK are not set")
	}
	c, err := sovos.New(sovos.Config{
		Username: user, Password: pass, VKN: vkn, Test: true,
		SenderAlias: os.Getenv("SOVOS_GB"), PostboxAlias: os.Getenv("SOVOS_PK"),
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	page, err := c.ListInbox(ctx, provider.ListQuery{})
	if err != nil {
		t.Fatalf("ListInbox: %v", err)
	}
	t.Logf("ListInbox: %d items", len(page.Items))
	if id := os.Getenv("SOVOS_STATUS_UUID"); id != "" {
		st, err := c.Status(ctx, provider.StatusQuery{Kind: provider.EInvoice, UUID: id})
		if err != nil {
			t.Fatalf("Status: %v", err)
		}
		t.Logf("Status: %s %s %s", st.Status, st.ProviderCode, st.Message)
	}
	if id := os.Getenv("SOVOS_EARCHIVE_UUID"); id != "" {
		st, err := c.Status(ctx, provider.StatusQuery{Kind: provider.EArchive, UUID: id})
		if err != nil {
			t.Fatalf("e-Arşiv Status: %v", err)
		}
		t.Logf("e-Arşiv Status: %s %s %s", st.Status, st.ProviderCode, st.Message)
	}
}
