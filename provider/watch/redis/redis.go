// Package redisstore implements the stores, locker and sinks of
// provider/watch on Redis, so several processes can share poller state.
//
//	rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
//	st := redisstore.New(rdb, redisstore.Options{})
//	ip, _ := watch.NewInboxPoller(p, watch.InboxConfig{Cursors: st, Dedup: st})
//	r := &watch.Runner{Sink: &redisstore.StreamSink{Client: rdb, Stream: "ubltr:events"}, Locker: st}
//
// Keys live under Options.Prefix (default "ubltr:watch:"): cursor:<key>
// (string), seen:<id> (string with TTL), watch:<key> (hash of UUID to
// JSON item) and lock:<key> (string with TTL).
package redisstore

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/furkanmeclis/go-ubltr/provider/watch"
)

// DefaultPrefix is the key prefix used when Options.Prefix is empty.
const DefaultPrefix = "ubltr:watch:"

// Options configures a Store.
type Options struct {
	// Prefix is prepended to every key (default DefaultPrefix).
	Prefix string
}

// Store is a watch.CursorStore, watch.DedupStore, watch.WatchList and
// watch.Locker backed by Redis. It works with *redis.Client,
// *redis.ClusterClient and *redis.Ring.
type Store struct {
	c      redis.UniversalClient
	prefix string
}

// New returns a Store using c.
func New(c redis.UniversalClient, opts Options) *Store {
	p := opts.Prefix
	if p == "" {
		p = DefaultPrefix
	}
	return &Store{c: c, prefix: p}
}

func (s *Store) key(kind, k string) string { return s.prefix + kind + ":" + k }

// Load implements watch.CursorStore.
func (s *Store) Load(ctx context.Context, key string) (string, error) {
	v, err := s.c.Get(ctx, s.key("cursor", key)).Result()
	if errors.Is(err, redis.Nil) {
		return "", nil
	}
	return v, err
}

// Save implements watch.CursorStore.
func (s *Store) Save(ctx context.Context, key, value string) error {
	return s.c.Set(ctx, s.key("cursor", key), value, 0).Err()
}

// Seen implements watch.DedupStore with SET NX EX.
func (s *Store) Seen(ctx context.Context, id string, ttl time.Duration) (bool, error) {
	ok, err := s.c.SetNX(ctx, s.key("seen", id), 1, ttl).Result()
	if err != nil {
		return false, err
	}
	return !ok, nil
}

// Forget implements watch.DedupStore.
func (s *Store) Forget(ctx context.Context, id string) error {
	return s.c.Del(ctx, s.key("seen", id)).Err()
}

// Add implements watch.WatchList.
func (s *Store) Add(ctx context.Context, key string, item watch.WatchItem) error {
	if item.UUID == "" {
		return errors.New("redisstore: watch item without UUID")
	}
	b, err := json.Marshal(item)
	if err != nil {
		return err
	}
	return s.c.HSet(ctx, s.key("watch", key), item.UUID, b).Err()
}

// Remove implements watch.WatchList.
func (s *Store) Remove(ctx context.Context, key, uuid string) error {
	return s.c.HDel(ctx, s.key("watch", key), uuid).Err()
}

// List implements watch.WatchList. Items are ordered by AddedAt, then UUID.
func (s *Store) List(ctx context.Context, key string) ([]watch.WatchItem, error) {
	m, err := s.c.HGetAll(ctx, s.key("watch", key)).Result()
	if err != nil {
		return nil, err
	}
	out := make([]watch.WatchItem, 0, len(m))
	for uuid, raw := range m {
		var it watch.WatchItem
		if err := json.Unmarshal([]byte(raw), &it); err != nil {
			return nil, fmt.Errorf("redisstore: watch item %s: %w", uuid, err)
		}
		out = append(out, it)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].AddedAt.Equal(out[j].AddedAt) {
			return out[i].UUID < out[j].UUID
		}
		return out[i].AddedAt.Before(out[j].AddedAt)
	})
	return out, nil
}

// unlockScript deletes the lock only when it still holds our token.
var unlockScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
	return redis.call("DEL", KEYS[1])
end
return 0
`)

// Lock implements watch.Locker with SET NX PX and a random token. Unlock
// runs a compare-and-delete script so an expired holder never releases a
// lease taken over by someone else.
func (s *Store) Lock(ctx context.Context, key string, ttl time.Duration) (func(context.Context) error, error) {
	if ttl < time.Millisecond {
		return nil, errors.New("redisstore: lock ttl below 1ms")
	}
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return nil, err
	}
	token := hex.EncodeToString(buf[:])
	k := s.key("lock", key)
	err := s.c.Do(ctx, "SET", k, token, "NX", "PX", ttl.Milliseconds()).Err()
	if errors.Is(err, redis.Nil) {
		return nil, watch.ErrLocked
	}
	if err != nil {
		return nil, err
	}
	return func(ctx context.Context) error {
		return unlockScript.Run(ctx, s.c, []string{k}, token).Err()
	}, nil
}

// StreamSink appends events to a Redis stream with XADD. Entries have
// the fields id, type and event (the JSON body). Consumers read them with
// XREAD or consumer groups and acknowledge with XACK.
type StreamSink struct {
	Client redis.UniversalClient
	Stream string
	// MaxLen trims the stream approximately (MAXLEN ~) when > 0.
	MaxLen int64
}

// Publish implements watch.Sink.
func (s *StreamSink) Publish(ctx context.Context, ev watch.Event) error {
	b, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	return s.Client.XAdd(ctx, &redis.XAddArgs{
		Stream: s.Stream,
		MaxLen: s.MaxLen,
		Approx: s.MaxLen > 0,
		Values: []any{"id", ev.ID, "type", string(ev.Type), "event", b},
	}).Err()
}

// PubSubSink publishes the event JSON on a channel. Pub/Sub is fire and
// forget: subscribers that are offline miss events, so prefer StreamSink
// when delivery matters.
type PubSubSink struct {
	Client  redis.UniversalClient
	Channel string
}

// Publish implements watch.Sink.
func (s *PubSubSink) Publish(ctx context.Context, ev watch.Event) error {
	b, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	return s.Client.Publish(ctx, s.Channel, b).Err()
}

var (
	_ watch.CursorStore = (*Store)(nil)
	_ watch.DedupStore  = (*Store)(nil)
	_ watch.WatchList   = (*Store)(nil)
	_ watch.Locker      = (*Store)(nil)
	_ watch.Sink        = (*StreamSink)(nil)
	_ watch.Sink        = (*PubSubSink)(nil)
)
