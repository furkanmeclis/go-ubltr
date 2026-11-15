package watch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// Store is a [CursorStore], [DedupStore] and [WatchList] kept in memory
// and, when opened with [OpenFileStore], mirrored to a JSON file. It is
// safe for concurrent use within one process. A file store must not be
// shared by several processes; use the Redis or SQL modules for that.
type Store struct {
	mu   sync.Mutex
	st   storeState
	now  func() time.Time
	path string // empty for memory stores
}

type storeState struct {
	Cursors map[string]string               `json:"cursors"`
	Seen    map[string]time.Time            `json:"seen"`
	Watch   map[string]map[string]WatchItem `json:"watch"`
}

func newState() storeState {
	return storeState{
		Cursors: map[string]string{},
		Seen:    map[string]time.Time{},
		Watch:   map[string]map[string]WatchItem{},
	}
}

// NewMemoryStore returns an empty in-memory store. State is lost when the
// process exits, so restarts replay events from the initial lookback.
func NewMemoryStore() *Store {
	return &Store{st: newState(), now: time.Now}
}

// OpenFileStore loads or creates a JSON state file. Every change is
// written to a temporary file in the same directory and renamed over
// path, so the file is never half-written.
func OpenFileStore(path string) (*Store, error) {
	s := &Store{st: newState(), now: time.Now, path: path}
	data, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return s, nil
	case err != nil:
		return nil, fmt.Errorf("watch: open state: %w", err)
	}
	if len(data) == 0 {
		return s, nil
	}
	if err := json.Unmarshal(data, &s.st); err != nil {
		return nil, fmt.Errorf("watch: parse state %s: %w", path, err)
	}
	if s.st.Cursors == nil {
		s.st.Cursors = map[string]string{}
	}
	if s.st.Seen == nil {
		s.st.Seen = map[string]time.Time{}
	}
	if s.st.Watch == nil {
		s.st.Watch = map[string]map[string]WatchItem{}
	}
	return s, nil
}

// SetClock replaces time.Now (tests).
func (s *Store) SetClock(now func() time.Time) {
	s.mu.Lock()
	s.now = now
	s.mu.Unlock()
}

// Load implements CursorStore.
func (s *Store) Load(_ context.Context, key string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.st.Cursors[key], nil
}

// Save implements CursorStore.
func (s *Store) Save(_ context.Context, key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	old, had := s.st.Cursors[key]
	s.st.Cursors[key] = value
	return s.commit(func() {
		if had {
			s.st.Cursors[key] = old
		} else {
			delete(s.st.Cursors, key)
		}
	})
}

// Seen implements DedupStore.
func (s *Store) Seen(_ context.Context, id string, ttl time.Duration) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	if exp, ok := s.st.Seen[id]; ok && now.Before(exp) {
		return true, nil
	}
	old, had := s.st.Seen[id]
	s.st.Seen[id] = now.Add(ttl)
	err := s.commit(func() {
		if had {
			s.st.Seen[id] = old
		} else {
			delete(s.st.Seen, id)
		}
	})
	return false, err
}

// Forget implements DedupStore.
func (s *Store) Forget(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	old, had := s.st.Seen[id]
	if !had {
		return nil
	}
	delete(s.st.Seen, id)
	return s.commit(func() { s.st.Seen[id] = old })
}

// Add implements WatchList.
func (s *Store) Add(_ context.Context, key string, item WatchItem) error {
	if item.UUID == "" {
		return errors.New("watch: watch item without UUID")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	m := s.st.Watch[key]
	if m == nil {
		m = map[string]WatchItem{}
		s.st.Watch[key] = m
	}
	old, had := m[item.UUID]
	m[item.UUID] = item
	return s.commit(func() {
		if had {
			m[item.UUID] = old
		} else {
			delete(m, item.UUID)
		}
	})
}

// Remove implements WatchList.
func (s *Store) Remove(_ context.Context, key, uuid string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	m := s.st.Watch[key]
	old, had := m[uuid]
	if !had {
		return nil
	}
	delete(m, uuid)
	if len(m) == 0 {
		delete(s.st.Watch, key)
	}
	return s.commit(func() {
		if s.st.Watch[key] == nil {
			s.st.Watch[key] = m
		}
		m[uuid] = old
	})
}

// List implements WatchList. Items are ordered by AddedAt, then UUID.
func (s *Store) List(_ context.Context, key string) ([]WatchItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]WatchItem, 0, len(s.st.Watch[key]))
	for _, it := range s.st.Watch[key] {
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

// commit persists the state, calling undo when that fails. Caller holds mu.
func (s *Store) commit(undo func()) error {
	if s.path == "" {
		return nil
	}
	now := s.now()
	for id, exp := range s.st.Seen {
		if !now.Before(exp) {
			delete(s.st.Seen, id)
		}
	}
	if err := writeFileAtomic(s.path, s.st); err != nil {
		undo()
		return err
	}
	return nil
}

func writeFileAtomic(path string, v any) (err error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("watch: encode state: %w", err)
	}
	dir, base := filepath.Split(path)
	if dir == "" {
		dir = "."
	}
	f, err := os.CreateTemp(dir, base+".*.tmp")
	if err != nil {
		return fmt.Errorf("watch: write state: %w", err)
	}
	tmp := f.Name()
	defer func() {
		if err != nil {
			_ = os.Remove(tmp)
		}
	}()
	if _, err = f.Write(append(data, '\n')); err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return fmt.Errorf("watch: write state: %w", err)
	}
	if err = os.Rename(tmp, path); err != nil {
		return fmt.Errorf("watch: write state: %w", err)
	}
	return nil
}

// MemoryLocker is an in-process [Locker].
type MemoryLocker struct {
	mu    sync.Mutex
	held  map[string]memLease
	seq   uint64
	clock func() time.Time
}

type memLease struct {
	token   uint64
	expires time.Time
}

// NewMemoryLocker returns an empty MemoryLocker.
func NewMemoryLocker() *MemoryLocker {
	return &MemoryLocker{held: map[string]memLease{}, clock: time.Now}
}

// Lock implements Locker.
func (l *MemoryLocker) Lock(_ context.Context, key string, ttl time.Duration) (func(context.Context) error, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.clock()
	if cur, ok := l.held[key]; ok && now.Before(cur.expires) {
		return nil, ErrLocked
	}
	l.seq++
	token := l.seq
	l.held[key] = memLease{token: token, expires: now.Add(ttl)}
	return func(context.Context) error {
		l.mu.Lock()
		defer l.mu.Unlock()
		if cur, ok := l.held[key]; ok && cur.token == token {
			delete(l.held, key)
		}
		return nil
	}, nil
}

var (
	_ CursorStore = (*Store)(nil)
	_ DedupStore  = (*Store)(nil)
	_ WatchList   = (*Store)(nil)
	_ Locker      = (*MemoryLocker)(nil)
)
