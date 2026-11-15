//go:build live

package trendyol_test

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/furkanmeclis/go-ubltr/provider"
	"github.com/furkanmeclis/go-ubltr/provider/trendyol"
)

// Live tests run against the Trendyol stage gateway (IP allow-listed by
// Trendyol) with an account issued by Trendyol:
//
//	TRENDYOL_EMAIL, TRENDYOL_PASSWORD               signIn user (required)
//	TRENDYOL_USER_ID, TRENDYOL_COMPANY_ID           API user ids
//	TRENDYOL_CUSTOMER_EMAIL, _PASSWORD, _TAX_ID     marketplace mode instead
//	TRENDYOL_CHECK_VKN                              VKN for CheckUser (optional)
//
//	go test -tags live -run Live ./...
func TestLiveReadOnly(t *testing.T) {
	email, pass := os.Getenv("TRENDYOL_EMAIL"), os.Getenv("TRENDYOL_PASSWORD")
	if email == "" || pass == "" {
		t.Skip("TRENDYOL_EMAIL and TRENDYOL_PASSWORD are not set")
	}
	uid, _ := strconv.ParseInt(os.Getenv("TRENDYOL_USER_ID"), 10, 64)
	cid, _ := strconv.ParseInt(os.Getenv("TRENDYOL_COMPANY_ID"), 10, 64)
	c, err := trendyol.New(trendyol.Config{
		Email: email, Password: pass, UserID: uid, CompanyID: cid, Test: true,
		CustomerEmail:    os.Getenv("TRENDYOL_CUSTOMER_EMAIL"),
		CustomerPassword: os.Getenv("TRENDYOL_CUSTOMER_PASSWORD"),
		CustomerTaxID:    os.Getenv("TRENDYOL_CUSTOMER_TAX_ID"),
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	page, err := c.ListInbox(ctx, provider.ListQuery{From: time.Now().AddDate(0, 0, -7), Limit: 5})
	if err != nil {
		t.Fatalf("ListInbox: %v", err)
	}
	t.Logf("ListInbox: %d items, next %q", len(page.Items), page.Next)
	if vkn := os.Getenv("TRENDYOL_CHECK_VKN"); vkn != "" {
		u, err := c.CheckUser(ctx, vkn)
		if err != nil {
			t.Fatalf("CheckUser: %v", err)
		}
		t.Logf("CheckUser: %+v", u)
	}
}
