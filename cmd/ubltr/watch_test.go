package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/go-ubltr/provider/watch"
)

func readEvents(t *testing.T, s string) []watch.Event {
	t.Helper()
	var out []watch.Event
	err := watch.Pipe(context.Background(), watch.ReadEvents(strings.NewReader(s)), watch.FuncSink(func(_ context.Context, ev watch.Event) error {
		out = append(out, ev)
		return nil
	}))
	if err != nil {
		t.Fatalf("parse NDJSON: %v\n%s", err, s)
	}
	return out
}

func TestWatchOnceFake(t *testing.T) {
	t.Parallel()

	state := filepath.Join(t.TempDir(), "state.json")
	args := []string{"watch", "--provider", "fake", "--option", "seed=2", "--vkn", "1111111111", "--state", state, "--once"}
	var out, errBuf bytes.Buffer
	if code := run(args, &out, &errBuf); code != 0 {
		t.Fatalf("exit %d, stderr=%s", code, errBuf.String())
	}
	evs := readEvents(t, out.String())
	if len(evs) != 2 {
		t.Fatalf("events = %d:\n%s", len(evs), out.String())
	}
	if evs[0].Type != watch.EventInvoiceReceived || evs[0].Tenant != "1111111111" || evs[0].Provider != "fake" {
		t.Fatalf("event = %+v", evs[0])
	}

	// Same state file: already delivered, nothing printed.
	out.Reset()
	if code := run(args, &out, &errBuf); code != 0 {
		t.Fatalf("exit %d, stderr=%s", code, errBuf.String())
	}
	if out.Len() != 0 {
		t.Fatalf("second run printed:\n%s", out.String())
	}
}

func TestWatchExec(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("no sh")
	}
	dir := t.TempDir()
	log := filepath.Join(dir, "events.log")
	var out, errBuf bytes.Buffer
	code := run([]string{"watch", "--provider", "fake", "--option", "seed=3", "--once", "--exec", "cat >> " + log}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("exit %d, stderr=%s", code, errBuf.String())
	}
	data, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(readEvents(t, string(data))); n != 3 {
		t.Fatalf("handler saw %d events", n)
	}
	if out.Len() != 0 {
		t.Fatalf("stdout should be empty with --exec: %q", out.String())
	}

	code = run([]string{"watch", "--provider", "fake", "--option", "seed=1", "--once", "--exec", "exit 1"}, &out, &errBuf)
	if code == 0 {
		t.Fatal("failing handler: exit 0")
	}
}

func TestWatchTrack(t *testing.T) {
	t.Parallel()

	var out, errBuf bytes.Buffer
	// The fake does not know this UUID, so it stays tracked and nothing
	// is printed; the pass still succeeds.
	code := run([]string{"watch", "--provider", "fake", "--once", "--track", "efatura:00000000-0000-4000-8000-0000000000aa:TICARIFATURA"}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("exit %d, stderr=%s", code, errBuf.String())
	}
	if _, err := parseTrack("bogus:x"); err == nil {
		t.Fatal("bad kind accepted")
	}
	if _, err := parseTrack("a:b:c:d"); err == nil {
		t.Fatal("too many parts accepted")
	}
	if it, err := parseTrack("earsiv:u1"); err != nil || it.Kind != "earsiv" || it.UUID != "u1" {
		t.Fatalf("parseTrack = %+v, %v", it, err)
	}
}

func TestWatchRunStops(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	var out, errBuf bytes.Buffer
	code := runWatch(ctx, []string{"--provider", "fake", "--option", "seed=1", "--interval", "50ms"}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("exit %d, stderr=%s", code, errBuf.String())
	}
	if n := len(readEvents(t, out.String())); n != 1 {
		t.Fatalf("events = %d", n)
	}
}

func TestWatchUsageErrors(t *testing.T) {
	t.Parallel()

	cases := [][]string{
		{"watch"},
		{"watch", "--provider", "nope"},
		{"watch", "--provider", "fake", "--option", "novalue"},
		{"watch", "--provider", "fake", "--option", "seed=x"},
		{"watch", "--provider", "fake", "extra"},
	}
	for _, args := range cases {
		var out, errBuf bytes.Buffer
		if code := run(args, &out, &errBuf); code == 0 {
			t.Errorf("%v: exit 0", args)
		}
		if strings.Contains(errBuf.String(), "fake,") {
			t.Errorf("%v: hidden provider listed: %s", args, errBuf.String())
		}
	}
}
