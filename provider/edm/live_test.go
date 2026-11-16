//go:build live

package edm_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/furkanmeclis/go-ubltr/provider"
	"github.com/furkanmeclis/go-ubltr/provider/edm"
)

// Live tests call EDM with your own credentials. They only read: CheckUser,
// a header-only outbox listing (read documents included, so nothing is
// marked) and, when EDM_UUID is set, a status query.
//
//	EDM_USERNAME=... EDM_PASSWORD=... go test -tags live -run Live ./...
//
// EDM_ENDPOINT overrides the service URL (default: edm.TestEndpoint) and
// EDM_VKN is the VKN queried by CheckUser and sent as the sender VKN;
// TestLiveCheckUser skips without it.
func liveProvider(t *testing.T) *edm.Provider {
	t.Helper()
	user, pass := os.Getenv("EDM_USERNAME"), os.Getenv("EDM_PASSWORD")
	if user == "" || pass == "" {
		t.Skip("EDM_USERNAME / EDM_PASSWORD not set")
	}
	p, err := edm.New(edm.Config{Username: user, Password: pass, Endpoint: os.Getenv("EDM_ENDPOINT"), Test: true, VKN: os.Getenv("EDM_VKN")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = p.Logout(context.Background()) })
	return p
}

func TestLiveCheckUser(t *testing.T) {
	vkn := os.Getenv("EDM_VKN")
	if vkn == "" {
		t.Skip("EDM_VKN not set")
	}
	p := liveProvider(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	u, err := p.CheckUser(ctx, vkn)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("registered=%v title=%q aliases=%d", u.Registered, u.Title, len(u.Aliases))
}

func TestLiveListOutbox(t *testing.T) {
	p := liveProvider(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	page, err := p.ListOutbox(ctx, provider.ListQuery{Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range page.Items {
		t.Logf("%s %s %s %s sender=%s receiver=%s", it.ID, it.UUID, it.Status, it.ProviderCode, it.SenderVKN, it.ReceiverVKN)
	}
}

func TestLiveStatus(t *testing.T) {
	uuid := os.Getenv("EDM_UUID")
	if uuid == "" {
		t.Skip("EDM_UUID not set")
	}
	p := liveProvider(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	st, err := p.Status(ctx, provider.StatusQuery{UUID: uuid})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%+v", st)
}
