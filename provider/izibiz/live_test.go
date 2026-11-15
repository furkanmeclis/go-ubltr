//go:build live

package izibiz_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/furkanmeclis/go-ubltr/provider"
	"github.com/furkanmeclis/go-ubltr/provider/izibiz"
)

// Live tests call the İzibiz system with your own credentials. They only
// read: CheckUser, a header-only outbox listing and, when IZIBIZ_UUID is
// set, a status query.
//
//	IZIBIZ_USERNAME=... IZIBIZ_PASSWORD=... go test -tags live -run Live ./...
//
// IZIBIZ_ENDPOINT overrides the base URL (default: the test system).
func liveProvider(t *testing.T) *izibiz.Provider {
	t.Helper()
	user, pass := os.Getenv("IZIBIZ_USERNAME"), os.Getenv("IZIBIZ_PASSWORD")
	if user == "" || pass == "" {
		t.Skip("IZIBIZ_USERNAME / IZIBIZ_PASSWORD not set")
	}
	p, err := izibiz.New(izibiz.Config{Username: user, Password: pass, Endpoint: os.Getenv("IZIBIZ_ENDPOINT"), Test: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = p.Logout(context.Background()) })
	return p
}

func TestLiveCheckUser(t *testing.T) {
	p := liveProvider(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	// 4840847211 is the test account VKN published on the Developer Portal.
	u, err := p.CheckUser(ctx, "4840847211")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("registered=%v title=%q aliases=%d", u.Registered, u.Title, len(u.Aliases))
}

func TestLiveListOutbox(t *testing.T) {
	p := liveProvider(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	page, err := p.ListOutbox(ctx, provider.ListQuery{Limit: 5, From: time.Now().AddDate(0, 0, -30), To: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range page.Items {
		t.Logf("%s %s %s %s", it.ID, it.UUID, it.Status, it.ProviderCode)
	}
}

func TestLiveStatus(t *testing.T) {
	uuid := os.Getenv("IZIBIZ_UUID")
	if uuid == "" {
		t.Skip("IZIBIZ_UUID not set")
	}
	p := liveProvider(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	st, err := p.Status(ctx, provider.StatusQuery{Kind: provider.EInvoice, UUID: uuid})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%+v", st)
}
