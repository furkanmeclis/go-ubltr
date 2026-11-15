package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime"
	"slices"
	"strings"
	"syscall"

	"github.com/furkanmeclis/go-ubltr/provider"
	"github.com/furkanmeclis/go-ubltr/provider/watch"
)

const watchUsage = `Usage: ubltr watch --provider NAME [options]

Polls an integrator and prints one JSON event per line (NDJSON) to stdout:
incoming documents (invoice.received) and status changes of tracked
outgoing documents (document.status_changed, response.received,
earchive.cancelled). Stop with Ctrl-C; a pass in flight is finished first.

  --provider NAME     integrator module name (required)
  --username U        web-service user
  --password P        web-service password
  --api-key K         REST API key
  --endpoint URL      override the base URL
  --test              use the integrator's test system
  --vkn VKN           account VKN/TCKN
  --option k=v        integrator-specific option (repeatable)
  --tenant NAME       tenant name in events and state keys (default: --vkn)
  --interval D        poll interval, e.g. 30s, 5m (default 5m; raised to the
                      integrator's minimum)
  --state FILE        keep cursors, dedup and tracked documents in FILE
                      (default: memory only, replays the lookback on restart)
  --lookback D        how far back the first pass looks (default 24h)
  --ack               mark listed documents taken once handled, and ask only
                      for documents not yet taken (when supported)
  --track [KIND:]UUID[:PROFILE]
                      follow an outgoing document (repeatable); KIND is
                      efatura (default) or earsiv, PROFILE e.g. TICARIFATURA
  --exec CMD          run CMD for every event with the JSON on stdin
                      instead of printing it; a non-zero exit retries it
  --once              run one pass and exit (cron style)

Integrators: %s

Delivery is at least once: deduplicate on the event "id" field.
`

// optionFlags collects repeated --option k=v.
type optionFlags map[string]string

func (o optionFlags) String() string { return "" }

func (o optionFlags) Set(v string) error {
	k, val, ok := strings.Cut(v, "=")
	if !ok || k == "" {
		return fmt.Errorf("want k=v, got %q", v)
	}
	o[k] = val
	return nil
}

// listFlag collects a repeated string flag.
type listFlag []string

func (l *listFlag) String() string { return strings.Join(*l, ",") }

func (l *listFlag) Set(v string) error { *l = append(*l, v); return nil }

func cmdWatch(args []string, stdout, stderr io.Writer) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return runWatch(ctx, args, stdout, stderr)
}

func runWatch(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("watch", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var cfg provider.Config
	opts := optionFlags{}
	var tracks listFlag
	name := fs.String("provider", "", "integrator name")
	fs.StringVar(&cfg.Username, "username", "", "web-service user")
	fs.StringVar(&cfg.Password, "password", "", "web-service password")
	fs.StringVar(&cfg.APIKey, "api-key", "", "REST API key")
	fs.StringVar(&cfg.Endpoint, "endpoint", "", "base URL override")
	fs.BoolVar(&cfg.Test, "test", false, "use the test system")
	fs.StringVar(&cfg.VKN, "vkn", "", "account VKN/TCKN")
	fs.Var(opts, "option", "integrator option k=v")
	tenant := fs.String("tenant", "", "tenant name")
	interval := fs.Duration("interval", watch.DefaultInterval, "poll interval")
	statePath := fs.String("state", "", "state file")
	lookback := fs.Duration("lookback", watch.DefaultLookback, "first-pass lookback")
	ack := fs.Bool("ack", false, "mark documents taken")
	fs.Var(&tracks, "track", "outgoing document to follow")
	execCmd := fs.String("exec", "", "command run per event")
	once := fs.Bool("once", false, "one pass, then exit")
	fs.Usage = func() {
		list := strings.Join(publicProviders(), ", ")
		if list == "" {
			list = "none built into this binary yet"
		}
		fmt.Fprintf(stderr, watchUsage, list)
	}
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if *name == "" || fs.NArg() > 0 {
		fs.Usage()
		return 1
	}
	fail := func(err error) int {
		fmt.Fprintf(stderr, "ubltr watch: %v\n", err)
		return 1
	}
	cfg.Options = opts
	p, err := provider.Open(*name, cfg)
	if err != nil {
		return fail(err)
	}
	if *tenant == "" {
		*tenant = cfg.VKN
	}

	var store *watch.Store
	if *statePath != "" {
		if store, err = watch.OpenFileStore(*statePath); err != nil {
			return fail(err)
		}
	} else {
		store = watch.NewMemoryStore()
	}

	var sink watch.Sink = watch.NewWriterSink(stdout)
	if *execCmd != "" {
		sink = shellSink(*execCmd, stdout, stderr)
	}
	r := &watch.Runner{
		Sink: sink,
		OnError: func(j watch.Job, err error) {
			fmt.Fprintf(stderr, "ubltr watch: %s: %v\n", j.Key(), err)
		},
	}

	jobs := 0
	if _, ok := p.(provider.InboxLister); ok && hasOp(p, provider.OpListInbox) {
		ip, err := watch.NewInboxPoller(p, watch.InboxConfig{
			Tenant: *tenant, Cursors: store, Dedup: store, Lookback: *lookback,
			OnlyNew: *ack, AckAfterPublish: *ack,
		})
		if err != nil {
			return fail(err)
		}
		if err := r.Add(watch.Job{Name: "inbox", Provider: p, Tenant: *tenant, Source: ip, Interval: *interval}); err != nil {
			return fail(err)
		}
		jobs++
	}
	if _, ok := p.(provider.StatusChecker); ok && hasOp(p, provider.OpStatus) {
		tr, err := watch.NewOutboxTracker(p, watch.OutboxConfig{Tenant: *tenant, WatchList: store})
		if err != nil {
			return fail(err)
		}
		for _, t := range tracks {
			item, err := parseTrack(t)
			if err != nil {
				return fail(err)
			}
			if err := tr.Track(ctx, item); err != nil {
				return fail(err)
			}
		}
		if err := r.Add(watch.Job{Name: "outbox", Provider: p, Tenant: *tenant, Source: tr, Interval: *interval}); err != nil {
			return fail(err)
		}
		jobs++
	} else if len(tracks) > 0 {
		return fail(provider.Unsupported(p.Name(), provider.OpStatus, "--track"))
	}
	if jobs == 0 {
		return fail(fmt.Errorf("integrator %s supports neither inbox listing nor status queries", p.Name()))
	}

	if *once {
		if err := r.RunOnce(ctx); err != nil {
			return 1
		}
		return 0
	}
	if err := r.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		return fail(err)
	}
	return 0
}

func hasOp(p provider.Provider, op provider.Operation) bool {
	c := p.Capabilities()
	for _, k := range c.Kinds() {
		if c.Has(k, op) {
			return true
		}
	}
	return false
}

// parseTrack reads [KIND:]UUID[:PROFILE].
func parseTrack(s string) (watch.WatchItem, error) {
	parts := strings.Split(s, ":")
	item := watch.WatchItem{Kind: provider.EInvoice}
	switch len(parts) {
	case 1:
		item.UUID = parts[0]
	case 2:
		item.Kind, item.UUID = provider.DocumentKind(parts[0]), parts[1]
	case 3:
		item.Kind, item.UUID, item.Profile = provider.DocumentKind(parts[0]), parts[1], parts[2]
	default:
		return item, fmt.Errorf("--track %q: want [KIND:]UUID[:PROFILE]", s)
	}
	kinds := []string{string(provider.EArchive), string(provider.EInvoice), string(provider.EDespatch)}
	if item.UUID == "" || !slices.Contains(kinds, string(item.Kind)) {
		return item, fmt.Errorf("--track %q: want [KIND:]UUID[:PROFILE] with KIND one of %s", s, strings.Join(kinds, ", "))
	}
	return item, nil
}

// shellSink runs cmd through the platform shell for every event.
func shellSink(cmd string, stdout, stderr io.Writer) *watch.ExecSink {
	if runtime.GOOS == "windows" {
		return &watch.ExecSink{Path: "cmd", Args: []string{"/C", cmd}, Stdout: stdout, Stderr: stderr}
	}
	return &watch.ExecSink{Path: "/bin/sh", Args: []string{"-c", cmd}, Stdout: stdout, Stderr: stderr}
}
