//go:build live

package nes

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/furkanmeclis/go-ubltr/provider"
)

// Live tests run against the NES test system:
//
//	NES_API_KEY=... go test -tags live ./...
//
// Optional: NES_ENDPOINT, NES_SENDER_ALIAS, NES_CHECK_VKN, NES_SEND_XML (path
// to a UBL-TR file to upload; it is sent for real on the test system).
func liveClient(t *testing.T) *Client {
	t.Helper()
	key := os.Getenv("NES_API_KEY")
	if key == "" {
		t.Skip("NES_API_KEY not set")
	}
	c, err := New(Config{APIKey: key, Test: true, Endpoint: os.Getenv("NES_ENDPOINT"), SenderAlias: os.Getenv("NES_SENDER_ALIAS")})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func liveCtx(t *testing.T) context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)
	return ctx
}

func TestLiveListInbox(t *testing.T) {
	c := liveClient(t)
	page, err := c.ListInbox(liveCtx(t), provider.ListQuery{Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%d incoming, next=%q", len(page.Items), page.Next)
}

func TestLiveListOutboxEArchive(t *testing.T) {
	c := liveClient(t)
	page, err := c.ListOutbox(liveCtx(t), provider.ListQuery{Kind: provider.EArchive, Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%d e-Arşiv, next=%q", len(page.Items), page.Next)
}

func TestLiveCheckUser(t *testing.T) {
	vkn := os.Getenv("NES_CHECK_VKN")
	if vkn == "" {
		t.Skip("NES_CHECK_VKN not set")
	}
	u, err := liveClient(t).CheckUser(liveCtx(t), vkn)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%+v", u)
}

func TestLiveCredits(t *testing.T) {
	ci, err := liveClient(t).Credits(liveCtx(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%+v", ci)
}

func TestLiveSend(t *testing.T) {
	path := os.Getenv("NES_SEND_XML")
	if path == "" {
		t.Skip("NES_SEND_XML not set")
	}
	xml, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	c := liveClient(t)
	doc := &provider.Document{XML: xml}
	res, err := c.Send(liveCtx(t), doc)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%+v", res)
	st, err := c.Status(liveCtx(t), provider.StatusQuery{Kind: doc.Kind, UUID: res.UUID})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%+v", st)
}
