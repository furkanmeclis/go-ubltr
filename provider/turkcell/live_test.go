//go:build live

package turkcell

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/furkanmeclis/go-ubltr/provider"
)

// Live tests run against https://efaturaservicetest.isim360.com:
//
//	TURKCELL_API_KEY=... go test -tags live ./...
//
// Optional: TURKCELL_ENDPOINT, TURKCELL_STATUS_UUID (an outgoing e-Fatura
// ETTN to query), TURKCELL_SEND_XML (path to a UBL-TR file; it is sent for
// real on the test system). The inbox listing is limited to 6 calls per
// 15 minutes.
func liveClient(t *testing.T) *Client {
	t.Helper()
	key := os.Getenv("TURKCELL_API_KEY")
	if key == "" {
		t.Skip("TURKCELL_API_KEY not set")
	}
	c, err := New(Config{APIKey: key, Test: true, Endpoint: os.Getenv("TURKCELL_ENDPOINT")})
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

func TestLiveStatus(t *testing.T) {
	id := os.Getenv("TURKCELL_STATUS_UUID")
	if id == "" {
		t.Skip("TURKCELL_STATUS_UUID not set")
	}
	st, err := liveClient(t).Status(liveCtx(t), provider.StatusQuery{UUID: id})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%+v", st)
}

func TestLiveSend(t *testing.T) {
	path := os.Getenv("TURKCELL_SEND_XML")
	if path == "" {
		t.Skip("TURKCELL_SEND_XML not set")
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
