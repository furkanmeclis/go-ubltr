package watch

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/furkanmeclis/go-ubltr/provider"
)

// waitGoroutines fails t when the goroutine count does not return to base.
func waitGoroutines(t *testing.T, base int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for runtime.NumGoroutine() > base {
		if time.Now().After(deadline) {
			buf := make([]byte, 1<<16)
			n := runtime.Stack(buf, true)
			t.Fatalf("goroutine leak: %d > %d\n%s", runtime.NumGoroutine(), base, buf[:n])
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// Runner tests are not parallel so goroutine counts are meaningful.

func TestRunnerDeliversAndStops(t *testing.T) {
	base := runtime.NumGoroutine()
	f := newFake(nil)
	ip, err := NewInboxPoller(f, einvoiceOnly(InboxConfig{}))
	if err != nil {
		t.Fatal(err)
	}
	ch := make(chan Event, 16)
	r := &Runner{Sink: ChanSink(ch), Jitter: -1}
	if err := r.Add(Job{Name: "inbox", Provider: f, Source: ip, Interval: 10 * time.Millisecond}); err != nil {
		t.Fatal(err)
	}
	if err := r.Add(Job{Name: "inbox", Provider: f, Source: ip}); err == nil {
		t.Fatal("duplicate job accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- r.Run(ctx) }()

	deliver(f, provider.EInvoice, "a", time.Now())
	deliver(f, provider.EInvoice, "b", time.Now())
	got := map[string]bool{}
	timeout := time.After(5 * time.Second)
	for len(got) < 2 {
		select {
		case ev := <-ch:
			got[ev.UUID] = true
		case <-timeout:
			t.Fatalf("events = %v", got)
		}
	}
	if err := r.Add(Job{Name: "x", Provider: f, Source: ip}); err == nil {
		t.Fatal("Add while running accepted")
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not stop")
	}
	waitGoroutines(t, base)
}

func TestRunnerBackoffAndErrors(t *testing.T) {
	base := runtime.NumGoroutine()
	var calls atomic.Int32
	retryable := SourceFunc(func(context.Context, func(Event) error) error {
		calls.Add(1)
		return &provider.Error{Kind: provider.KindRateLimit}
	})
	var plainCalls atomic.Int32
	plain := SourceFunc(func(context.Context, func(Event) error) error {
		plainCalls.Add(1)
		return errors.New("boom")
	})
	var mu sync.Mutex
	reported := map[string]int{}
	f := newFake(nil)
	r := &Runner{
		Sink: &collector{}, Jitter: -1, MaxBackoff: time.Second,
		OnError: func(j Job, err error) {
			mu.Lock()
			reported[j.Name]++
			mu.Unlock()
		},
	}
	_ = r.Add(Job{Name: "retry", Provider: f, Source: retryable, Interval: 20 * time.Millisecond})
	_ = r.Add(Job{Name: "plain", Provider: f, Source: plain, Interval: 20 * time.Millisecond})
	ctx, cancel := context.WithTimeout(context.Background(), 330*time.Millisecond)
	defer cancel()
	if err := r.Run(ctx); err != nil {
		t.Fatal(err)
	}
	// Backoff: passes at 0, 40, 120, 280 ms -> about 4. Plain: every 20 ms.
	if n := calls.Load(); n < 2 || n > 6 {
		t.Fatalf("retryable passes = %d, want about 4", n)
	}
	if n := plainCalls.Load(); n < 8 {
		t.Fatalf("plain passes = %d, want >= 8", n)
	}
	mu.Lock()
	if reported["retry"] == 0 || reported["plain"] == 0 {
		t.Fatalf("reported = %v", reported)
	}
	mu.Unlock()
	waitGoroutines(t, base)

	if d := r.backoff(time.Minute, 10); d != time.Minute {
		t.Fatalf("backoff below interval: %v", d)
	}
	r.MaxBackoff = 0
	if d := r.backoff(time.Minute, 3); d != 8*time.Minute {
		t.Fatalf("backoff = %v", d)
	}
	if d := r.backoff(time.Minute, 50); d != DefaultMaxBackoff {
		t.Fatalf("backoff cap = %v", d)
	}
	r.Jitter = 0.5
	for range 100 {
		if d := r.jitter(time.Second); d < 500*time.Millisecond || d > 1500*time.Millisecond {
			t.Fatalf("jitter = %v", d)
		}
	}
}

func TestRunnerMinPollInterval(t *testing.T) {
	f := newFake(nil)
	caps := f.Capabilities()
	caps.MinPollInterval = time.Hour
	p := capsProvider{Fake: f, caps: caps}
	if d := (Job{Provider: p, Interval: time.Minute}).interval(); d != time.Hour {
		t.Fatalf("interval = %v", d)
	}
	if d := (Job{Provider: f}).interval(); d != DefaultInterval {
		t.Fatalf("default interval = %v", d)
	}
	if k := (Job{Provider: f, Tenant: "t", Name: "inbox"}).Key(); k != "fake|t|inbox" {
		t.Fatalf("key = %q", k)
	}
}

func TestRunnerLocker(t *testing.T) {
	ctx := context.Background()
	f := newFake(nil)
	var calls atomic.Int32
	src := SourceFunc(func(context.Context, func(Event) error) error { calls.Add(1); return nil })
	locker := NewMemoryLocker()
	job := Job{Name: "inbox", Provider: f, Tenant: "t", Source: src}

	unlock, err := locker.Lock(ctx, "watch|"+job.Key(), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	r := &Runner{Sink: &collector{}, Locker: locker}
	_ = r.Add(job)
	if err := r.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 0 {
		t.Fatal("pass ran while another holder had the lock")
	}
	_ = unlock(ctx)
	if err := r.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Fatalf("calls = %d", calls.Load())
	}
	// The lock is released after the pass.
	u, err := locker.Lock(ctx, "watch|"+job.Key(), time.Minute)
	if err != nil {
		t.Fatalf("lock not released: %v", err)
	}
	_ = u(ctx)

	// Two runners sharing a locker never overlap passes.
	var active, overlap atomic.Int32
	slow := SourceFunc(func(context.Context, func(Event) error) error {
		if active.Add(1) > 1 {
			overlap.Add(1)
		}
		time.Sleep(5 * time.Millisecond)
		active.Add(-1)
		return nil
	})
	cctx, cancel := context.WithTimeout(ctx, 150*time.Millisecond)
	defer cancel()
	var wg sync.WaitGroup
	for range 2 {
		rr := &Runner{Sink: &collector{}, Locker: locker, Jitter: -1}
		_ = rr.Add(Job{Name: "shared", Provider: f, Source: slow, Interval: time.Millisecond})
		wg.Go(func() { _ = rr.Run(cctx) })
	}
	wg.Wait()
	if overlap.Load() != 0 {
		t.Fatalf("overlapping passes: %d", overlap.Load())
	}
}

func TestRunnerDrain(t *testing.T) {
	base := runtime.NumGoroutine()
	f := newFake(nil)
	started := make(chan struct{})
	finishing := SourceFunc(func(ctx context.Context, emit func(Event) error) error {
		close(started)
		time.Sleep(50 * time.Millisecond)
		return emit(Event{ID: "late"})
	})
	sink := &collector{}
	r := &Runner{Sink: sink, DrainTimeout: 2 * time.Second}
	_ = r.Add(Job{Name: "a", Provider: f, Source: finishing, Interval: time.Hour})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- r.Run(ctx) }()
	<-started
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if len(sink.Events()) != 1 {
		t.Fatal("pass in flight was not drained")
	}

	// A pass that ignores the deadline is cancelled after DrainTimeout.
	started2 := make(chan struct{})
	stuck := SourceFunc(func(ctx context.Context, _ func(Event) error) error {
		close(started2)
		<-ctx.Done()
		return ctx.Err()
	})
	r2 := &Runner{Sink: sink, DrainTimeout: 20 * time.Millisecond}
	_ = r2.Add(Job{Name: "b", Provider: f, Source: stuck, Interval: time.Hour})
	ctx2, cancel2 := context.WithCancel(context.Background())
	go func() { done <- r2.Run(ctx2) }()
	<-started2
	cancel2()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("drain timeout ignored")
	}
	waitGoroutines(t, base)
}

func TestRunnerConfigErrors(t *testing.T) {
	ctx := context.Background()
	if err := (&Runner{}).Run(ctx); err == nil {
		t.Fatal("Run without sink")
	}
	if err := (&Runner{Sink: &collector{}}).Run(ctx); err == nil {
		t.Fatal("Run without jobs")
	}
	r := &Runner{}
	if err := r.Add(Job{}); err == nil {
		t.Fatal("job without source")
	}
	if err := r.Add(Job{Source: SourceFunc(nil)}); err == nil {
		t.Fatal("job without provider")
	}
	f := newFake(nil)
	r = &Runner{Sink: &collector{}}
	_ = r.Add(Job{Name: "e", Provider: f, Source: SourceFunc(func(context.Context, func(Event) error) error { return errors.New("x") })})
	if err := r.RunOnce(ctx); err == nil {
		t.Fatal("RunOnce hid the error")
	}
}
