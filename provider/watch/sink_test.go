package watch

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/furkanmeclis/go-ubltr/provider"
)

func sampleEvent(uuid string) Event {
	return Event{
		ID: EventID("fake", "", provider.Inbound, uuid, provider.StatusReceived), Type: EventInvoiceReceived,
		Provider: "fake", Kind: provider.EInvoice, Direction: provider.Inbound, UUID: uuid,
		Status: provider.StatusReceived, At: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
	}
}

func TestWriterSinkReadEvents(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	var buf bytes.Buffer
	bw := bufio.NewWriter(&buf)
	s := NewWriterSink(bw)
	for _, u := range []string{"a", "b"} {
		if err := s.Publish(ctx, sampleEvent(u)); err != nil {
			t.Fatal(err)
		}
	}
	// Flushed per event: nothing left buffered.
	if bw.Buffered() != 0 {
		t.Fatal("WriterSink did not flush")
	}
	if n := strings.Count(buf.String(), "\n"); n != 2 {
		t.Fatalf("lines = %d: %q", n, buf.String())
	}
	in := buf.String() + "\n  \n"
	c := &collector{}
	if err := Pipe(ctx, ReadEvents(strings.NewReader(in)), c); err != nil {
		t.Fatal(err)
	}
	if got := c.Events(); len(got) != 2 || got[1] != sampleEvent("b") {
		t.Fatalf("read back = %+v", got)
	}
	if err := Pipe(ctx, ReadEvents(strings.NewReader("{bad\n")), c); err == nil {
		t.Fatal("bad line accepted")
	}
	if err := Pipe(ctx, ReadEvents(strings.NewReader(in)), &collector{failAt: 1}); err == nil {
		t.Fatal("sink error swallowed")
	}
}

func TestSmallSinks(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	var n atomic.Int32
	fs := FuncSink(func(context.Context, Event) error { n.Add(1); return nil })
	failing := FuncSink(func(context.Context, Event) error { return errors.New("down") })
	if err := (MultiSink{fs, fs}).Publish(ctx, sampleEvent("a")); err != nil || n.Load() != 2 {
		t.Fatalf("multi = %v, n = %d", err, n.Load())
	}
	if err := (MultiSink{fs, failing, fs}).Publish(ctx, sampleEvent("a")); err == nil || n.Load() != 4 {
		t.Fatalf("multi with failure = %v, n = %d", err, n.Load())
	}

	ch := make(chan Event, 1)
	if err := ChanSink(ch).Publish(ctx, sampleEvent("a")); err != nil {
		t.Fatal(err)
	}
	cctx, cancel := context.WithCancel(ctx)
	cancel()
	if err := ChanSink(ch).Publish(cctx, sampleEvent("b")); !errors.Is(err, context.Canceled) {
		t.Fatalf("full channel err = %v", err)
	}
}

func TestHTTPSink(t *testing.T) {
	t.Parallel()

	secret := []byte("s3cret")
	var calls atomic.Int32
	var lastErr atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		body, _ := io.ReadAll(r.Body)
		if err := VerifySignature(secret, r.Header.Get(HeaderSignature), r.Header.Get(HeaderTimestamp), body, 5*time.Minute, time.Now()); err != nil {
			lastErr.Store(err.Error())
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.Header.Get(HeaderEventType) != string(EventInvoiceReceived) || r.Header.Get("X-Extra") != "1" {
			lastErr.Store("headers")
		}
		switch r.URL.Path {
		case "/flaky":
			if n == 1 {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
		case "/bad":
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	ctx := context.Background()
	retry := provider.RetryPolicy{Attempts: 3, Base: time.Millisecond, Max: 5 * time.Millisecond}
	s := &HTTPSink{URL: srv.URL + "/flaky", Secret: secret, Retry: retry, Header: http.Header{"X-Extra": {"1"}}, Client: srv.Client()}
	if err := s.Publish(ctx, sampleEvent("a")); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 {
		t.Fatalf("calls = %d, want 2 (one retry)", calls.Load())
	}
	if v := lastErr.Load(); v != nil {
		t.Fatalf("server saw: %v", v)
	}
	calls.Store(0)
	bad := &HTTPSink{URL: srv.URL + "/bad", Secret: secret, Retry: retry, Header: http.Header{"X-Extra": {"1"}}}
	err := bad.Publish(ctx, sampleEvent("a"))
	if !errors.Is(err, provider.ErrInvalidRequest) || calls.Load() != 1 {
		t.Fatalf("bad = %v after %d calls", err, calls.Load())
	}
	wrong := &HTTPSink{URL: srv.URL + "/ok", Secret: []byte("other"), Retry: retry}
	if err := wrong.Publish(ctx, sampleEvent("a")); !errors.Is(err, provider.ErrAuth) {
		t.Fatalf("wrong secret = %v", err)
	}
	if err := (&HTTPSink{}).Publish(ctx, sampleEvent("a")); err == nil {
		t.Fatal("missing URL accepted")
	}
}

func TestVerifySignature(t *testing.T) {
	t.Parallel()

	secret, body := []byte("k"), []byte(`{"id":"x"}`)
	now := time.Unix(1_700_000_000, 0)
	ts := "1700000000"
	sig := Sign(secret, ts, body)
	if !strings.HasPrefix(sig, "sha256=") || len(sig) != len("sha256=")+64 {
		t.Fatalf("sig = %q", sig)
	}
	if err := VerifySignature(secret, sig, ts, body, time.Minute, now); err != nil {
		t.Fatal(err)
	}
	if err := VerifySignature(secret, sig, ts, []byte("tampered"), 0, now); !errors.Is(err, ErrBadSignature) {
		t.Fatalf("tampered = %v", err)
	}
	if err := VerifySignature(secret, sig, ts, body, time.Minute, now.Add(time.Hour)); !errors.Is(err, ErrBadSignature) {
		t.Fatalf("stale = %v", err)
	}
	if err := VerifySignature(secret, "md5=00", ts, body, 0, now); !errors.Is(err, ErrBadSignature) {
		t.Fatalf("scheme = %v", err)
	}
}

func TestExecSink(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh")
	}
	dir := t.TempDir()
	out := filepath.Join(dir, "out")
	s := &ExecSink{Path: sh, Args: []string{"-c", `cat > "$OUT"; printf '%s %s' "$UBLTR_EVENT_TYPE" "$UBLTR_UUID" > "$OUT.env"`}, Env: []string{"OUT=" + out}}
	if err := s.Publish(context.Background(), sampleEvent("u1")); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(out)
	if !strings.Contains(string(data), `"uuid":"u1"`) || !strings.HasSuffix(string(data), "\n") {
		t.Fatalf("stdin = %q", data)
	}
	env, _ := os.ReadFile(out + ".env")
	if string(env) != "invoice.received u1" {
		t.Fatalf("env = %q", env)
	}
	fail := &ExecSink{Path: sh, Args: []string{"-c", "exit 3"}}
	if err := fail.Publish(context.Background(), sampleEvent("u1")); err == nil {
		t.Fatal("non-zero exit accepted")
	}
	if err := (&ExecSink{}).Publish(context.Background(), sampleEvent("u1")); err == nil {
		t.Fatal("empty command accepted")
	}
}
