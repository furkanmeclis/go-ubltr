# Watching integrators (webhook replacement)

Most özel entegratör APIs cannot call you back. `provider/watch` polls them
and turns the results into events, so the rest of your system can react as
if it had webhooks. It is an opt-in module; the core library and `provider`
do not depend on it.

```bash
go get github.com/furkanmeclis/go-ubltr/provider/watch          # stdlib only
go get github.com/furkanmeclis/go-ubltr/provider/watch/redis    # Redis state and sinks
go get github.com/furkanmeclis/go-ubltr/provider/watch/sql      # database/sql state and outbox
```

The module README, [`provider/watch/README.md`](../provider/watch/README.md),
has the full design. This page is the short how-to.

## Events

| Type | When |
| --- | --- |
| `invoice.received` | A new incoming e-Fatura / e-Arşiv invoice is listed |
| `document.received` | A new incoming document of another kind (e-İrsaliye, …) |
| `document.status_changed` | A tracked outgoing document changed status |
| `response.received` | A tracked TICARIFATURA got KABUL or RED |
| `earchive.cancelled` | A tracked e-Arşiv invoice was cancelled |

Each event is one JSON object with stable snake_case fields. `id` is
`provider|tenant|direction|uuid|status`: use it as the idempotency key,
because delivery is **at least once**.

## From the command line

`ubltr watch` prints events as NDJSON on stdout:

```bash
ubltr watch --provider NAME --username U --password P --vkn VKN \
  [--test] [--endpoint URL] [--option k=v] \
  [--interval 5m] [--state FILE] [--ack] [--track [KIND:]UUID[:PROFILE]] \
  [--exec CMD] [--once]
```

| Flag | Meaning |
| --- | --- |
| `--interval` | Poll interval; raised to the integrator's minimum |
| `--state FILE` | Keep cursor, dedup and tracked documents across runs (JSON, atomic writes) |
| `--ack` | Mark documents taken after handling and list only new ones, when supported |
| `--track` | Follow an outgoing document's status (repeatable) |
| `--exec CMD` | Run `CMD` (via `sh -c`) per event with the JSON on stdin instead of printing |
| `--once` | One pass and exit, for cron |

Examples:

```bash
ubltr watch --provider NAME ... --state watch.json | jq 'select(.type=="invoice.received")'
ubltr watch --provider NAME ... --state watch.json --exec ./handle.sh
ubltr watch --provider NAME ... --state watch.json --once >> events.ndjson
```

A handler that exits non-zero leaves the event unacknowledged; it comes
back on the next pass. Handlers also get `UBLTR_EVENT_ID`,
`UBLTR_EVENT_TYPE`, `UBLTR_UUID`, `UBLTR_STATUS` and friends in the
environment.

The CLI opens integrators through `provider.Open`, so only integrator
modules compiled into the binary are available (`cmd/ubltr/providers.go`).

## From Go

```go
st, _ := watch.OpenFileStore("watch.json")
inbox, _ := watch.NewInboxPoller(p, watch.InboxConfig{Tenant: vkn, Cursors: st, Dedup: st})
r := &watch.Runner{Sink: &watch.HTTPSink{URL: hookURL, Secret: secret}}
_ = r.Add(watch.Job{Name: "inbox", Provider: p, Tenant: vkn, Source: inbox})
_ = r.Run(ctx)
```

Choosing a sink:

| Sink | Use |
| --- | --- |
| `WriterSink` | NDJSON to stdout or a file (the default of the CLI) |
| `ExecSink` | A script per event |
| `HTTPSink` | Your own webhook endpoint, HMAC-SHA256 signed (`X-Ubltr-Signature`) |
| `ChanSink`, `FuncSink` | In-process handling |
| `MultiSink` | Fan out; fails when any sink fails |
| `redisstore.StreamSink` | Redis stream for consumer groups |
| `sqlstore.OutboxSink` | Transactional outbox table |

Choosing state:

| Store | Use |
| --- | --- |
| `NewMemoryStore` | Tests, throwaway runs (replays the lookback on restart) |
| `OpenFileStore` | One process on one machine |
| `redisstore.Store` | Several replicas; also a `Locker` |
| `sqlstore.Store` | Several replicas with an existing database; also a `Locker` |

With several replicas, set `Runner.Locker` so only one of them polls each
account at a time.

## Delivery guarantees

- Cursors advance only after the sink accepted every event of a page.
- A pass lists from the last watermark minus an overlap window (15 minutes);
  duplicates from the overlap are dropped by the dedup store.
- Retryable integrator errors and rate limits back the job off
  exponentially (capped at one hour); other errors are reported and retried
  at the normal interval.
- Cancelling the context lets a pass in flight finish (30 s drain by
  default).
