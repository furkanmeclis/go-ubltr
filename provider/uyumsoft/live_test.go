//go:build live

package uyumsoft

// Live tests against the vendor test systems. Run by the maintainer:
//
//	UYUMSOFT_USERNAME=... UYUMSOFT_PASSWORD=... go test -tags live -run Live -v ./...
//
// Optional:
//
//	UYUMSOFT_ENDPOINT          override the test endpoint
//	UYUMSOFT_LIVE_VKN          VKN/TCKN for CheckUser (default: none, skipped)
//	MUHASEBEIZLEME_USERNAME    muhasebeizleme user (informative)
//	MUHASEBEIZLEME_API_KEY     muhasebeizleme test API key
//
// The tests only read: they never send, answer or cancel documents.

import (
	"os"
	"testing"
	"time"

	"github.com/furkanmeclis/go-ubltr/provider"
)

func liveClient(t *testing.T, p Preset, userEnv, passEnv string) *Client {
	t.Helper()
	user, pass := os.Getenv(userEnv), os.Getenv(passEnv)
	if pass == "" || (p.Auth == AuthWSSecurity && user == "") {
		t.Skipf("%s / %s not set", userEnv, passEnv)
	}
	cfg := Config{Username: user, Password: pass, Preset: p, Test: true}
	if p.Name == Uyumsoft.Name {
		cfg.Endpoint = os.Getenv("UYUMSOFT_ENDPOINT")
	}
	c, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func runLive(t *testing.T, c *Client) {
	if err := c.Ping(ctx); err != nil {
		t.Fatalf("TestConnection: %v", err)
	}
	if ci, err := c.Credits(ctx); err != nil {
		t.Errorf("Credits: %v", err)
	} else {
		t.Logf("credits: %+v", ci)
	}
	from := time.Now().AddDate(0, 0, -30)
	out, err := c.ListOutbox(ctx, provider.ListQuery{From: from, Limit: 5})
	if err != nil {
		t.Fatalf("ListOutbox: %v", err)
	}
	t.Logf("outbox: %d items, next %q", len(out.Items), out.Next)
	if len(out.Items) > 0 {
		d := out.Items[0]
		st, err := c.Status(ctx, provider.StatusQuery{Kind: d.Kind, UUID: d.UUID})
		if err != nil {
			t.Errorf("Status(%s): %v", d.UUID, err)
		} else {
			t.Logf("status: %s (%s)", st.Status, st.ProviderCode)
		}
		for _, f := range []provider.Format{provider.FormatUBL, provider.FormatPDF, provider.FormatHTML} {
			r, err := c.Fetch(ctx, provider.FetchQuery{Kind: d.Kind, UUID: d.UUID, Format: f})
			if err != nil {
				t.Errorf("Fetch %s: %v", f, err)
				continue
			}
			t.Logf("fetch %s: %d bytes", f, len(r.Data))
		}
	}
	in, err := c.ListInbox(ctx, provider.ListQuery{From: from, Limit: 5})
	if err != nil {
		t.Fatalf("ListInbox: %v", err)
	}
	t.Logf("inbox: %d items", len(in.Items))
	if vkn := os.Getenv("UYUMSOFT_LIVE_VKN"); vkn != "" {
		u, err := c.CheckUser(ctx, vkn)
		if err != nil {
			t.Errorf("CheckUser: %v", err)
		} else {
			t.Logf("user: %+v", u)
		}
	}
}

func TestLiveUyumsoft(t *testing.T) {
	runLive(t, liveClient(t, Uyumsoft, "UYUMSOFT_USERNAME", "UYUMSOFT_PASSWORD"))
}

func TestLiveMuhasebeizleme(t *testing.T) {
	runLive(t, liveClient(t, Muhasebeizleme, "MUHASEBEIZLEME_USERNAME", "MUHASEBEIZLEME_API_KEY"))
}
