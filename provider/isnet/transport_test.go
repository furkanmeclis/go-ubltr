package isnet_test

import (
	"context"
	"errors"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/furkanmeclis/go-ubltr/provider"
	"github.com/furkanmeclis/go-ubltr/provider/isnet"
)

// blackhole accepts TCP connections and never answers, like an endpoint
// that silently drops a caller.
func blackhole(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var conns []net.Conn
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			conns = append(conns, c)
			mu.Unlock()
		}
	}()
	t.Cleanup(func() {
		_ = ln.Close()
		<-done
		mu.Lock()
		defer mu.Unlock()
		for _, c := range conns {
			_ = c.Close()
		}
	})
	return "http://" + ln.Addr().String() + "/InvoiceService.svc"
}

// hang is a Doer that never answers; it returns when the request context
// ends, the way net/http does.
type hang struct{ calls int }

func (h *hang) Do(r *http.Request) (*http.Response, error) {
	h.calls++
	<-r.Context().Done()
	return nil, &net.OpError{Op: "dial", Net: "tcp", Err: r.Context().Err()}
}

func checkIPError(t *testing.T, err error) {
	t.Helper()
	if !isnet.IsIPNotAllowed(err) || !errors.Is(err, isnet.ErrIPNotAllowed) {
		t.Fatalf("want ip-not-allowlisted, got %v", err)
	}
	var pe *provider.Error
	if !errors.As(err, &pe) || pe.Kind != provider.KindPermission || pe.Code != isnet.CodeIPNotAllowed || pe.Retryable ||
		pe.Message == "" || pe.Provider != isnet.Name || pe.Op != "SearchAllInvoice" {
		t.Fatalf("error fields %#v", pe)
	}
	if !errors.Is(err, provider.ErrPermission) || provider.IsRetryable(err) {
		t.Fatal("must match ErrPermission and not be retryable")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("cause must stay visible")
	}
}

func TestIPNotAllowedOnFilteredEndpoint(t *testing.T) {
	c, err := isnet.New(isnet.Config{VKN: vkn, Endpoint: blackhole(t), IPFiltered: true, Timeout: 200 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	_, err = c.ListInbox(context.Background(), provider.ListQuery{})
	checkIPError(t, err)
	if time.Since(start) > 5*time.Second {
		t.Fatal("timeout not applied")
	}
}

func TestTimeoutOnTestEndpoint(t *testing.T) {
	c, err := isnet.New(isnet.Config{VKN: vkn, Endpoint: blackhole(t), Timeout: 200 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.ListInbox(context.Background(), provider.ListQuery{})
	if isnet.IsIPNotAllowed(err) || provider.KindOf(err) != provider.KindTimeout || !provider.IsRetryable(err) {
		t.Fatalf("want a plain timeout, got %v", err)
	}
}

// The live host is IP-filtered without IPFiltered; the test host is not.
func TestDefaultEndpointsClassification(t *testing.T) {
	h := &hang{}
	live, err := isnet.New(isnet.Config{VKN: vkn, HTTP: h, Timeout: 50 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	_, err = live.ListOutbox(context.Background(), provider.ListQuery{})
	if !isnet.IsIPNotAllowed(err) {
		t.Fatalf("live endpoint: %v", err)
	}
	if _, err := live.CheckUser(context.Background(), "9876543210"); !isnet.IsIPNotAllowed(err) {
		t.Fatalf("live address book: %v", err)
	}
	test, err := isnet.New(isnet.Config{VKN: vkn, HTTP: h, Test: true, Timeout: 50 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	_, err = test.ListOutbox(context.Background(), provider.ListQuery{})
	if isnet.IsIPNotAllowed(err) || provider.KindOf(err) != provider.KindTimeout {
		t.Fatalf("test endpoint: %v", err)
	}
	if h.calls != 3 {
		t.Fatalf("calls %d", h.calls)
	}
}

// A caller's own deadline on the live endpoint is reported the same way:
// İşNet gives no other signal.
func TestCallerDeadlineOnLiveEndpoint(t *testing.T) {
	live, err := isnet.New(isnet.Config{VKN: vkn, HTTP: &hang{}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err = live.ListInbox(ctx, provider.ListQuery{})
	checkIPError(t, err)
}

func TestCancelAndRefusedAreNotIPErrors(t *testing.T) {
	live, err := isnet.New(isnet.Config{VKN: vkn, HTTP: &hang{}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(20 * time.Millisecond); cancel() }()
	if _, err := live.ListInbox(ctx, provider.ListQuery{}); isnet.IsIPNotAllowed(err) || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel: %v", err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	c, err := isnet.New(isnet.Config{VKN: vkn, Endpoint: "http://" + addr + "/x", IPFiltered: true, Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.ListInbox(context.Background(), provider.ListQuery{}); isnet.IsIPNotAllowed(err) || provider.KindOf(err) != provider.KindTransport {
		t.Fatalf("refused: %v", err)
	}
}
