package watch

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
)

// WriterSink writes events as NDJSON: one JSON object per line. It
// flushes after every event when the writer has a Flush method (such as
// *bufio.Writer), so a reader on the other end of a pipe sees each event
// as soon as it is published. It is safe for concurrent use.
type WriterSink struct {
	mu sync.Mutex
	w  io.Writer
}

// NewWriterSink returns a sink writing NDJSON to w.
func NewWriterSink(w io.Writer) *WriterSink { return &WriterSink{w: w} }

// Publish implements Sink.
func (s *WriterSink) Publish(_ context.Context, ev Event) error {
	b, err := json.Marshal(ev)
	if err != nil {
		return fmt.Errorf("watch: encode event: %w", err)
	}
	b = append(b, '\n')
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.w.Write(b); err != nil {
		return err
	}
	if f, ok := s.w.(interface{ Flush() error }); ok {
		return f.Flush()
	}
	return nil
}

// FuncSink adapts a function to [Sink].
type FuncSink func(ctx context.Context, ev Event) error

// Publish implements Sink.
func (f FuncSink) Publish(ctx context.Context, ev Event) error { return f(ctx, ev) }

// ChanSink sends events on a channel, blocking until the receiver takes
// the event or ctx ends.
type ChanSink chan<- Event

// Publish implements Sink.
func (c ChanSink) Publish(ctx context.Context, ev Event) error {
	select {
	case c <- ev:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// MultiSink publishes every event to each sink in order and fails when
// any of them fails. Sinks that succeeded see the event again when the
// source retries it, so each should be idempotent on [Event.ID].
type MultiSink []Sink

// Publish implements Sink.
func (m MultiSink) Publish(ctx context.Context, ev Event) error {
	var errs []error
	for _, s := range m {
		if err := s.Publish(ctx, ev); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// ReadEvents returns a Source that decodes NDJSON events from r until EOF,
// the counterpart of [WriterSink]. Blank lines are skipped.
func ReadEvents(r io.Reader) Source {
	return SourceFunc(func(ctx context.Context, emit func(Event) error) error {
		sc := bufio.NewScanner(r)
		sc.Buffer(make([]byte, 0, 64*1024), 16<<20)
		line := 0
		for sc.Scan() {
			line++
			if err := ctx.Err(); err != nil {
				return err
			}
			b := sc.Bytes()
			if len(bytes.TrimSpace(b)) == 0 {
				continue
			}
			var ev Event
			if err := json.Unmarshal(b, &ev); err != nil {
				return fmt.Errorf("watch: line %d: %w", line, err)
			}
			if err := emit(ev); err != nil {
				return err
			}
		}
		return sc.Err()
	})
}

var (
	_ Sink = (*WriterSink)(nil)
	_ Sink = FuncSink(nil)
	_ Sink = ChanSink(nil)
	_ Sink = MultiSink(nil)
)
