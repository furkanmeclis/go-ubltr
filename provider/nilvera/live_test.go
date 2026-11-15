//go:build live

package nilvera

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/furkanmeclis/go-ubltr/provider"
)

// Live tests run against https://apitest.nilvera.com:
//
//	NILVERA_API_KEY=... go test -tags live ./...
//
// Optional: NILVERA_ENDPOINT, NILVERA_CHECK_VKN, NILVERA_SEND_XML (path to a
// UBL-TR file; it is sent for real on the test system), NILVERA_ALIAS
// (receiver alias for NILVERA_SEND_XML).
func liveClient(t *testing.T) *Client {
	t.Helper()
	key := os.Getenv("NILVERA_API_KEY")
	if key == "" {
		t.Skip("NILVERA_API_KEY not set")
	}
	c, err := New(Config{APIKey: key, Test: true, Endpoint: os.Getenv("NILVERA_ENDPOINT")})
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
	page, err := liveClient(t).ListInbox(liveCtx(t), provider.ListQuery{Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%d incoming, next=%q", len(page.Items), page.Next)
}

func TestLiveListOutboxEArchive(t *testing.T) {
	page, err := liveClient(t).ListOutbox(liveCtx(t), provider.ListQuery{Kind: provider.EArchive, Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%d e-Arşiv, next=%q", len(page.Items), page.Next)
}

func TestLiveCheckUser(t *testing.T) {
	vkn := os.Getenv("NILVERA_CHECK_VKN")
	if vkn == "" {
		t.Skip("NILVERA_CHECK_VKN not set")
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
	path := os.Getenv("NILVERA_SEND_XML")
	if path == "" {
		t.Skip("NILVERA_SEND_XML not set")
	}
	xml, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	c := liveClient(t)
	doc := &provider.Document{XML: xml, ReceiverAlias: os.Getenv("NILVERA_ALIAS")}
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
