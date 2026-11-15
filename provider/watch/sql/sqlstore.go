// Package sqlstore implements the stores, locker and a transactional
// outbox sink of provider/watch on database/sql. It ships no driver: open
// the database with the driver of your choice and pick the matching
// [Dialect].
//
//	db, _ := sql.Open("pgx", dsn)
//	st, _ := sqlstore.New(db, sqlstore.Options{Dialect: sqlstore.Postgres})
//	_ = st.Migrate(ctx)
//	ip, _ := watch.NewInboxPoller(p, watch.InboxConfig{Cursors: st, Dedup: st})
//	r := &watch.Runner{Sink: st.Outbox(), Locker: st}
//
// The SQL is portable across PostgreSQL, MySQL / MariaDB and SQLite: no
// upsert syntax, times stored as BIGINT Unix milliseconds, and keys as
// VARCHAR(255).
package sqlstore

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/go-ubltr/provider/watch"
)

// Dialect captures the differences between databases that matter here.
type Dialect struct {
	Name string
	// Placeholder returns the n-th (1-based) bind parameter. Nil means "?".
	Placeholder func(n int) string
}

// Dialects.
var (
	Postgres = Dialect{Name: "postgres", Placeholder: func(n int) string { return "$" + strconv.Itoa(n) }}
	MySQL    = Dialect{Name: "mysql"}
	SQLite   = Dialect{Name: "sqlite"}
)

// rebind turns ? placeholders into the dialect's form.
func (d Dialect) rebind(q string) string {
	if d.Placeholder == nil {
		return q
	}
	var b strings.Builder
	n := 0
	for i := 0; i < len(q); i++ {
		if q[i] == '?' {
			n++
			b.WriteString(d.Placeholder(n))
			continue
		}
		b.WriteByte(q[i])
	}
	return b.String()
}

// DefaultPrefix is the table-name prefix used when Options.Prefix is
// empty.
const DefaultPrefix = "ubltr_watch_"

// Options configures a Store.
type Options struct {
	// Dialect selects placeholders (default "?", as for MySQL and SQLite).
	Dialect Dialect
	// Prefix is prepended to table names (default DefaultPrefix). Only
	// letters, digits and underscores are allowed.
	Prefix string
	// Now replaces time.Now (tests).
	Now func() time.Time
}

// Store is a watch.CursorStore, watch.DedupStore, watch.WatchList and
// watch.Locker backed by SQL tables. Call Migrate once to create them.
type Store struct {
	db  *sql.DB
	d   Dialect
	now func() time.Time

	cursors, dedup, watchlist, locks, outbox string
}

// New returns a Store using db.
func New(db *sql.DB, opts Options) (*Store, error) {
	if db == nil {
		return nil, errors.New("sqlstore: nil db")
	}
	p := opts.Prefix
	if p == "" {
		p = DefaultPrefix
	}
	for _, r := range p {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '_' {
			return nil, fmt.Errorf("sqlstore: invalid table prefix %q", p)
		}
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	return &Store{
		db: db, d: opts.Dialect, now: now,
		cursors: p + "cursors", dedup: p + "dedup", watchlist: p + "watchlist",
		locks: p + "locks", outbox: p + "outbox",
	}, nil
}

// Migrate creates the tables when they do not exist.
func (s *Store) Migrate(ctx context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS ` + s.cursors + ` (
	cursor_key VARCHAR(255) NOT NULL PRIMARY KEY,
	cursor_value TEXT NOT NULL,
	updated_at BIGINT NOT NULL
)`,
		`CREATE TABLE IF NOT EXISTS ` + s.dedup + ` (
	event_id VARCHAR(255) NOT NULL PRIMARY KEY,
	expires_at BIGINT NOT NULL
)`,
		`CREATE TABLE IF NOT EXISTS ` + s.watchlist + ` (
	list_key VARCHAR(255) NOT NULL,
	uuid VARCHAR(64) NOT NULL,
	item TEXT NOT NULL,
	added_at BIGINT NOT NULL,
	PRIMARY KEY (list_key, uuid)
)`,
		`CREATE TABLE IF NOT EXISTS ` + s.locks + ` (
	lock_key VARCHAR(255) NOT NULL PRIMARY KEY,
	owner VARCHAR(64) NOT NULL,
	expires_at BIGINT NOT NULL
)`,
		`CREATE TABLE IF NOT EXISTS ` + s.outbox + ` (
	event_id VARCHAR(255) NOT NULL PRIMARY KEY,
	event_type VARCHAR(64) NOT NULL,
	payload TEXT NOT NULL,
	created_at BIGINT NOT NULL,
	published_at BIGINT NULL
)`,
	}
	for _, q := range stmts {
		if _, err := s.db.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("sqlstore: migrate: %w", err)
		}
	}
	return nil
}

// querier is satisfied by *sql.DB and *sql.Tx.
type querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

func (s *Store) exec(ctx context.Context, q querier, query string, args ...any) (sql.Result, error) {
	return q.ExecContext(ctx, s.d.rebind(query), args...)
}

func (s *Store) exists(ctx context.Context, q querier, query string, args ...any) (bool, error) {
	var one int
	err := q.QueryRowContext(ctx, s.d.rebind(query), args...).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

func ms(t time.Time) int64 { return t.UnixMilli() }

// upsert runs update, then insert when no row matched. A failed insert
// is fine when the row exists by then (a concurrent writer, or MySQL
// reporting zero changed rows for identical values): update runs again.
func (s *Store) upsert(ctx context.Context, update string, uargs []any, insert string, iargs []any, check string, cargs []any) error {
	res, err := s.exec(ctx, s.db, update, uargs...)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err == nil && n > 0 {
		return nil
	}
	_, ierr := s.exec(ctx, s.db, insert, iargs...)
	if ierr == nil {
		return nil
	}
	ok, err := s.exists(ctx, s.db, check, cargs...)
	if err != nil || !ok {
		return ierr
	}
	_, err = s.exec(ctx, s.db, update, uargs...)
	return err
}

// Load implements watch.CursorStore.
func (s *Store) Load(ctx context.Context, key string) (string, error) {
	var v string
	err := s.db.QueryRowContext(ctx, s.d.rebind(`SELECT cursor_value FROM `+s.cursors+` WHERE cursor_key = ?`), key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return v, err
}

// Save implements watch.CursorStore.
func (s *Store) Save(ctx context.Context, key, value string) error {
	now := ms(s.now())
	return s.upsert(ctx,
		`UPDATE `+s.cursors+` SET cursor_value = ?, updated_at = ? WHERE cursor_key = ?`, []any{value, now, key},
		`INSERT INTO `+s.cursors+` (cursor_key, cursor_value, updated_at) VALUES (?, ?, ?)`, []any{key, value, now},
		`SELECT 1 FROM `+s.cursors+` WHERE cursor_key = ?`, []any{key})
}

// Seen implements watch.DedupStore. The primary key makes the insert
// the atomic check-and-set; expired rows are deleted first.
func (s *Store) Seen(ctx context.Context, id string, ttl time.Duration) (bool, error) {
	now := s.now()
	if _, err := s.exec(ctx, s.db, `DELETE FROM `+s.dedup+` WHERE event_id = ? AND expires_at <= ?`, id, ms(now)); err != nil {
		return false, err
	}
	_, ierr := s.exec(ctx, s.db, `INSERT INTO `+s.dedup+` (event_id, expires_at) VALUES (?, ?)`, id, ms(now.Add(ttl)))
	if ierr == nil {
		return false, nil
	}
	ok, err := s.exists(ctx, s.db, `SELECT 1 FROM `+s.dedup+` WHERE event_id = ?`, id)
	if err != nil || !ok {
		return false, ierr
	}
	return true, nil
}

// Forget implements watch.DedupStore.
func (s *Store) Forget(ctx context.Context, id string) error {
	_, err := s.exec(ctx, s.db, `DELETE FROM `+s.dedup+` WHERE event_id = ?`, id)
	return err
}

// Prune deletes expired dedup rows and locks. Run it now and then (for
// example daily); correctness does not depend on it.
func (s *Store) Prune(ctx context.Context) error {
	now := ms(s.now())
	if _, err := s.exec(ctx, s.db, `DELETE FROM `+s.dedup+` WHERE expires_at <= ?`, now); err != nil {
		return err
	}
	_, err := s.exec(ctx, s.db, `DELETE FROM `+s.locks+` WHERE expires_at <= ?`, now)
	return err
}

// Add implements watch.WatchList.
func (s *Store) Add(ctx context.Context, key string, item watch.WatchItem) error {
	if item.UUID == "" {
		return errors.New("sqlstore: watch item without UUID")
	}
	b, err := json.Marshal(item)
	if err != nil {
		return err
	}
	added := ms(item.AddedAt)
	return s.upsert(ctx,
		`UPDATE `+s.watchlist+` SET item = ?, added_at = ? WHERE list_key = ? AND uuid = ?`, []any{string(b), added, key, item.UUID},
		`INSERT INTO `+s.watchlist+` (list_key, uuid, item, added_at) VALUES (?, ?, ?, ?)`, []any{key, item.UUID, string(b), added},
		`SELECT 1 FROM `+s.watchlist+` WHERE list_key = ? AND uuid = ?`, []any{key, item.UUID})
}

// Remove implements watch.WatchList.
func (s *Store) Remove(ctx context.Context, key, uuid string) error {
	_, err := s.exec(ctx, s.db, `DELETE FROM `+s.watchlist+` WHERE list_key = ? AND uuid = ?`, key, uuid)
	return err
}

// List implements watch.WatchList, ordered by AddedAt then UUID.
func (s *Store) List(ctx context.Context, key string) ([]watch.WatchItem, error) {
	rows, err := s.db.QueryContext(ctx, s.d.rebind(`SELECT item FROM `+s.watchlist+` WHERE list_key = ? ORDER BY added_at, uuid`), key)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []watch.WatchItem
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var it watch.WatchItem
		if err := json.Unmarshal([]byte(raw), &it); err != nil {
			return nil, fmt.Errorf("sqlstore: watch item: %w", err)
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// Lock implements watch.Locker with a row lease: a row per key holding a
// random owner token and an expiry. An expired lease is taken over with a
// conditional UPDATE; a missing one is created with INSERT, which the
// primary key makes exclusive.
func (s *Store) Lock(ctx context.Context, key string, ttl time.Duration) (func(context.Context) error, error) {
	if ttl < time.Millisecond {
		return nil, errors.New("sqlstore: lock ttl below 1ms")
	}
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return nil, err
	}
	owner := hex.EncodeToString(buf[:])
	now := s.now()
	exp := ms(now.Add(ttl))
	res, err := s.exec(ctx, s.db, `UPDATE `+s.locks+` SET owner = ?, expires_at = ? WHERE lock_key = ? AND expires_at <= ?`, owner, exp, key, ms(now))
	if err != nil {
		return nil, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil, err
	}
	if n == 0 {
		if _, ierr := s.exec(ctx, s.db, `INSERT INTO `+s.locks+` (lock_key, owner, expires_at) VALUES (?, ?, ?)`, key, owner, exp); ierr != nil {
			held, err := s.exists(ctx, s.db, `SELECT 1 FROM `+s.locks+` WHERE lock_key = ?`, key)
			if err != nil || !held {
				return nil, ierr
			}
			return nil, watch.ErrLocked
		}
	}
	return func(ctx context.Context) error {
		_, err := s.exec(ctx, s.db, `DELETE FROM `+s.locks+` WHERE lock_key = ? AND owner = ?`, key, owner)
		return err
	}, nil
}

// Outbox returns the transactional outbox sink of this store.
func (s *Store) Outbox() *OutboxSink { return &OutboxSink{s: s} }

// OutboxSink writes events to the outbox table (event_id, event_type,
// payload, created_at, published_at). A relay process reads rows with
// Pending, delivers them and calls MarkPublished. Use PublishTx to write
// an event in the same transaction as your own data.
//
// Inserting an event ID that is already present is a no-op, so
// redelivered events are absorbed.
type OutboxSink struct{ s *Store }

// OutboxEntry is one outbox row.
type OutboxEntry struct {
	Event       watch.Event
	CreatedAt   time.Time
	PublishedAt time.Time
}

// Publish implements watch.Sink.
func (o *OutboxSink) Publish(ctx context.Context, ev watch.Event) error {
	return o.insert(ctx, o.s.db, ev)
}

// PublishTx writes ev inside tx. The event becomes visible to the relay
// only when tx commits.
func (o *OutboxSink) PublishTx(ctx context.Context, tx *sql.Tx, ev watch.Event) error {
	return o.insert(ctx, tx, ev)
}

func (o *OutboxSink) insert(ctx context.Context, q querier, ev watch.Event) error {
	s := o.s
	if ev.ID == "" {
		return errors.New("sqlstore: event without ID")
	}
	// Check first: a failed INSERT would abort a PostgreSQL transaction.
	ok, err := s.exists(ctx, q, `SELECT 1 FROM `+s.outbox+` WHERE event_id = ?`, ev.ID)
	if err != nil || ok {
		return err
	}
	b, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	_, err = s.exec(ctx, q, `INSERT INTO `+s.outbox+` (event_id, event_type, payload, created_at) VALUES (?, ?, ?, ?)`,
		ev.ID, string(ev.Type), string(b), ms(s.now()))
	if err != nil {
		if _, tx := q.(*sql.Tx); !tx {
			if ok, _ := s.exists(ctx, q, `SELECT 1 FROM `+s.outbox+` WHERE event_id = ?`, ev.ID); ok {
				return nil
			}
		}
	}
	return err
}

// Pending returns up to limit unpublished entries, oldest first.
func (o *OutboxSink) Pending(ctx context.Context, limit int) ([]OutboxEntry, error) {
	s := o.s
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, s.d.rebind(`SELECT payload, created_at FROM `+s.outbox+
		` WHERE published_at IS NULL ORDER BY created_at, event_id LIMIT `+strconv.Itoa(limit)))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []OutboxEntry
	for rows.Next() {
		var raw string
		var created int64
		if err := rows.Scan(&raw, &created); err != nil {
			return nil, err
		}
		e := OutboxEntry{CreatedAt: time.UnixMilli(created)}
		if err := json.Unmarshal([]byte(raw), &e.Event); err != nil {
			return nil, fmt.Errorf("sqlstore: outbox payload: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// MarkPublished records that the relay delivered ids.
func (o *OutboxSink) MarkPublished(ctx context.Context, ids ...string) error {
	s := o.s
	if len(ids) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	now := ms(s.now())
	for _, id := range ids {
		if _, err := s.exec(ctx, tx, `UPDATE `+s.outbox+` SET published_at = ? WHERE event_id = ?`, now, id); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

var (
	_ watch.CursorStore = (*Store)(nil)
	_ watch.DedupStore  = (*Store)(nil)
	_ watch.WatchList   = (*Store)(nil)
	_ watch.Locker      = (*Store)(nil)
	_ watch.Sink        = (*OutboxSink)(nil)
)
