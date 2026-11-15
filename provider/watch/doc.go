// Package watch turns integrator polling into events, for integrators
// that offer no webhooks.
//
// The model is small and Unix-like: a [Source] produces [Event] values and
// a [Sink] consumes them. [InboxPoller] lists incoming documents and emits
// [EventInvoiceReceived]; [OutboxTracker] follows outgoing documents kept
// on a [WatchList] and emits status changes. A [Runner] schedules sources
// per integrator and tenant, honours provider.Capabilities.MinPollInterval,
// backs off on transient errors and can hold a [Locker] so that only one
// process polls a given account.
//
// Delivery is at least once. Cursors advance only after the sink accepted
// every event of a page, so a crash or a failing sink leads to redelivery,
// never to loss. Every event carries a stable [Event.ID] built from the
// provider, tenant, direction, UUID and status; consumers use it as an
// idempotency key. A [DedupStore] suppresses most duplicates before they
// reach the sink.
//
// The default sink, [WriterSink], writes one JSON object per line
// (NDJSON), so events can be piped into jq, a shell script or another
// process that reads them back with [ReadEvents]. Durable stores and sinks
// for Redis and SQL live in the nested modules provider/watch/redis and
// provider/watch/sql.
//
// Nothing in the core library or in package provider depends on this
// package.
package watch
