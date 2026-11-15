//go:build live

package qnb_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/furkanmeclis/go-ubltr/provider"
	"github.com/furkanmeclis/go-ubltr/provider/qnb"
)

// Live tests run against the QNB test system with credentials issued by
// QNB:
//
//	QNB_USERNAME, QNB_PASSWORD, QNB_VKN   e-Fatura web-service user (required)
//	QNB_ERP_CODE                          erpKodu (optional)
//	QNB_EARCHIVE_USERNAME / _PASSWORD     e-Arşiv user (optional)
//	QNB_CHECK_VKN                         VKN for CheckUser (default QNB_VKN)
//	QNB_STATUS_UUID                       outgoing e-Fatura ETTN for Status (optional)
//
//	go test -tags live -run Live ./...
func liveClient(t *testing.T) *qnb.Client {
	t.Helper()
	user, pass, vkn := os.Getenv("QNB_USERNAME"), os.Getenv("QNB_PASSWORD"), os.Getenv("QNB_VKN")
	if user == "" || pass == "" || vkn == "" {
		t.Skip("QNB_USERNAME, QNB_PASSWORD and QNB_VKN are not set")
	}
	c, err := qnb.New(qnb.Config{
		Username: user, Password: pass, VKN: vkn, ERPCode: os.Getenv("QNB_ERP_CODE"),
		EArchiveUsername: os.Getenv("QNB_EARCHIVE_USERNAME"), EArchivePassword: os.Getenv("QNB_EARCHIVE_PASSWORD"),
		Test: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestLiveReadOnly(t *testing.T) {
	c := liveClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	vkn := os.Getenv("QNB_CHECK_VKN")
	if vkn == "" {
		vkn = os.Getenv("QNB_VKN")
	}
	u, err := c.CheckUser(ctx, vkn)
	if err != nil {
		t.Fatalf("CheckUser: %v", err)
	}
	t.Logf("CheckUser: %+v", u)
	page, err := c.ListInbox(ctx, provider.ListQuery{Limit: 5})
	if err != nil {
		t.Fatalf("ListInbox: %v", err)
	}
	t.Logf("ListInbox: %d items, next %q", len(page.Items), page.Next)
	if cr, err := c.Credits(ctx); err != nil {
		t.Logf("Credits: %v", err)
	} else {
		t.Logf("Credits: %+v", cr)
	}
	if id := os.Getenv("QNB_STATUS_UUID"); id != "" {
		st, err := c.Status(ctx, provider.StatusQuery{Kind: provider.EInvoice, UUID: id})
		if err != nil {
			t.Fatalf("Status: %v", err)
		}
		t.Logf("Status: %s %s %s", st.Status, st.ProviderCode, st.Message)
	}
}
