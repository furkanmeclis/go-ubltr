# provider/watch

Webhooks for integrators that have none. `watch` polls an özel entegratör
through the vendor-neutral [`provider`](../README.md) contract, turns what
it finds into events and hands them to a sink: stdout, a script, an HTTP
endpoint, Redis or an SQL outbox.

It is optional. Nothing in the core library or in `provider` depends on it,
and it has no third-party dependencies.

```bash
go get github.com/furkanmeclis/go-ubltr/provider/watch
```

## Design

```
 integrator ──poll──▶ Source ──Event──▶ Sink
                      │                  │
            InboxPoller / OutboxTracker  WriterSink (NDJSON), ExecSink,
            (cursor, dedup, watch list)  HTTPSink, ChanSink, MultiSink,
                                         Redis stream, SQL outbox …
```

| Piece | Role |
| --- | --- |
| `Event` | One change, JSON with stable snake_case names. `ID` = `provider\|tenant\|direction\|uuid\|status` is the idempotency key. |
| `Source` | `Run(ctx, emit)` — one pass for pollers, until EOF for streams. |
| `Sink` | `Publish(ctx, Event)` — returns nil only once the event is safe. |
| `InboxPoller` | `provider.InboxLister` → `invoice.received` (`document.received` for e-İrsaliye and others). |
| `OutboxTracker` | `provider.StatusChecker` over a `WatchList` → `document.status_changed`, `response.received` (KABUL / RED of a TICARIFATURA), `earchive.cancelled`. |
| `Runner` | Schedules jobs per (provider, tenant), interval ≥ `Capabilities.MinPollInterval`, jitter, exponential backoff on retryable and rate-limit errors, graceful drain, optional `Locker`. |
| `CursorStore`, `DedupStore`, `WatchList`, `Locker` | State. Memory and JSON-file implementations here; Redis and SQL in nested modules. |

### Event

```json
{"id":"uyumsoft|1234567890|inbound|f2b1…|received","type":"invoice.received",
 "provider":"uyumsoft","tenant":"1234567890","kind":"efatura","direction":"inbound",
 "uuid":"f2b1…","document_id":"ABC2026000000042","status":"received",
 "info":{"uuid":"f2b1…","id":"ABC2026000000042","profile":"TICARIFATURA","sender_vkn":"9876543210", …},
 "at":"2026-03-01T12:00:00Z"}
```

Types: `invoice.received`, `document.received`, `document.status_changed`,
`response.received`, `earchive.cancelled`. A status transition produces
exactly one event, so `id` stays unique per (document, status).

### Inbox cursor

Many integrator APIs filter only by date, so progress is a **time
watermark** per kind. A pass lists `[watermark − overlap, now]` (overlap
15 minutes by default, to catch late-indexed documents and clock skew);
the dedup store drops what the overlap lists again. The integrator's page
cursor is saved after every accepted page, so an interrupted pass resumes
where it stopped, and the watermark moves only when the whole pass was
accepted.

With `OnlyNew` (when the integrator supports `mark-taken`) the listing asks
for documents not yet taken; with `AckAfterPublish` the poller calls
`MarkTaken` for each page after the sink accepted it.

### Outbox tracking

Add outgoing documents to the watch list after sending:

```go
res, err := sender.Send(ctx, doc)
_ = tracker.TrackSent(ctx, doc, res)
```

A document leaves the list when its status is settled:
`Status.Terminal()` for TICARIFATURA (KABUL, RED, failure) or when the
response window (8 days after delivery by default) ends;
`Status.Settled()` for the rest, or `Terminal()` with `WaitTerminal`
(for example to see e-Arşiv cancellations). `MaxAge` (30 days) drops
documents that never settle.

## Delivery semantics

- **At least once.** Cursors and watch-list statuses advance only after the
  sink accepted the event. A crash, a failing sink or a failing
  acknowledgement means redelivery, never loss.
- **Deduplicate on `id`.** The `DedupStore` suppresses most repeats
  (overlap window, restarts), but sinks and consumers must still treat
  `id` as an idempotency key. `MultiSink` re-publishes to every sink when
  one of them failed.
- **Ordering** is per source pass; there is no global order across jobs.
- **One poller per account.** Give the `Runner` a shared `Locker` (Redis
  or SQL) when several replicas run; passes that find the lock held are
  skipped.
- **State.** `NewMemoryStore` forgets on restart (the first pass replays
  `Lookback`, 24h). `OpenFileStore` keeps state in a JSON file written with
  an atomic rename; it is for one process. Use Redis or SQL for more.

## Library use

```go
p, _ := provider.Open("uyumsoft", provider.Config{Username: u, Password: pw})
st, _ := watch.OpenFileStore("watch-state.json")

inbox, _ := watch.NewInboxPoller(p, watch.InboxConfig{Tenant: vkn, Cursors: st, Dedup: st})
outbox, _ := watch.NewOutboxTracker(p, watch.OutboxConfig{Tenant: vkn, WatchList: st})

r := &watch.Runner{
	Sink: watch.MultiSink{
		watch.NewWriterSink(os.Stdout),
		&watch.HTTPSink{URL: "https://erp.example.com/hooks/ubltr", Secret: secret},
	},
	OnError: func(j watch.Job, err error) { log.Printf("%s: %v", j.Key(), err) },
}
_ = r.Add(watch.Job{Name: "inbox", Provider: p, Tenant: vkn, Source: inbox, Interval: 5 * time.Minute})
_ = r.Add(watch.Job{Name: "outbox", Provider: p, Tenant: vkn, Source: outbox, Interval: 10 * time.Minute})
_ = r.Run(ctx) // until ctx is cancelled
```

`r.RunOnce(ctx)` runs one pass of every job, for cron.

### HTTP sink

`HTTPSink` POSTs the event JSON. With `Secret` set it sends

```
X-Ubltr-Timestamp: 1772366400
X-Ubltr-Signature: sha256=<hex HMAC-SHA256 of "<timestamp>.<body>">
X-Ubltr-Event-Id:  <event id>
X-Ubltr-Event-Type: invoice.received
```

The receiver checks it with `watch.VerifySignature(secret, sig, ts, body,
5*time.Minute, time.Now())`. 2xx is success; 408, 429, 5xx and network
errors are retried with `provider.Retry`.

## Unix pipes

`ubltr watch` (from [`cmd/ubltr`](../../cmd/ubltr)) prints NDJSON on
stdout, one event per line, flushed per event:

```bash
# Follow the inbox, show sender and number.
ubltr watch --provider uyumsoft --username u --password p --vkn 1234567890 \
  --state ~/.ubltr-watch.json | jq -r 'select(.type=="invoice.received") | "\(.info.sender_name)\t\(.document_id)"'

# Run a handler per event (JSON on stdin, UBLTR_* in the environment).
# A non-zero exit makes the event retry on the next pass.
ubltr watch --provider uyumsoft ... --exec ./handle.sh

# Cron: one pass, then exit.
*/10 * * * * ubltr watch --provider uyumsoft ... --state /var/lib/ubltr/watch.json --once >> /var/log/ubltr-events.ndjson
```

`handle.sh`:

```bash
#!/bin/sh
set -e
event=$(cat)
case "$UBLTR_EVENT_TYPE" in
  invoice.received)  echo "$event" | curl -fsS -H 'Content-Type: application/json' -d @- http://localhost:8080/inbox ;;
  response.received) echo "answer for $UBLTR_UUID: $UBLTR_STATUS" >&2 ;;
esac
```

Read events back in Go with `watch.ReadEvents(os.Stdin)`, which is a
`Source`:

```go
_ = watch.Pipe(ctx, watch.ReadEvents(os.Stdin), mySink)
```

## Redis

```bash
go get github.com/furkanmeclis/go-ubltr/provider/watch/redis
```

```go
rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
st := redisstore.New(rdb, redisstore.Options{}) // keys under "ubltr:watch:"

inbox, _ := watch.NewInboxPoller(p, watch.InboxConfig{Cursors: st, Dedup: st})
r := &watch.Runner{
	Sink:   &redisstore.StreamSink{Client: rdb, Stream: "ubltr:events", MaxLen: 100_000},
	Locker: st,
}
```

Dedup is `SET NX EX`, the lock is `SET NX PX` with a Lua compare-and-delete
unlock, the watch list is a hash per key. `StreamSink` writes `XADD … MAXLEN
~ N` with fields `id`, `type` and `event`; consume with consumer groups.
`PubSubSink` is fire and forget.

## SQL

```bash
go get github.com/furkanmeclis/go-ubltr/provider/watch/sql
```

```go
db, _ := sql.Open("pgx", dsn) // any database/sql driver
st, _ := sqlstore.New(db, sqlstore.Options{Dialect: sqlstore.Postgres})
_ = st.Migrate(ctx) // CREATE TABLE IF NOT EXISTS ubltr_watch_*

r := &watch.Runner{Sink: st.Outbox(), Locker: st}
```

Portable SQL for PostgreSQL (`$1`), MySQL / MariaDB and SQLite (`?`): no
upsert syntax, BIGINT Unix-millisecond times. The locker is a row lease
with expiry. `OutboxSink` is a transactional outbox: `PublishTx` writes an
event in your own transaction; a relay reads `Pending` and calls
`MarkPublished`. Call `Prune` now and then to delete expired dedup rows
and locks.

## Limitations

- The file store is for a single process.
- `Info` is filled only for inbox events (status queries return no listing
  metadata).
- The SQL outbox has no index on `published_at`; add one for large tables.
