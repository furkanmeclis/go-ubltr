package watch

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/furkanmeclis/go-ubltr/provider"
)

// Runner defaults.
const (
	DefaultInterval     = 5 * time.Minute
	DefaultJitter       = 0.1
	DefaultMaxBackoff   = time.Hour
	DefaultDrainTimeout = 30 * time.Second
)

// Job is one source polled on a schedule.
type Job struct {
	// Name tells several sources of the same account apart, e.g. "inbox"
	// and "outbox".
	Name string
	// Provider supplies the name used in keys and the minimum interval.
	Provider provider.Provider
	// Tenant identifies the account.
	Tenant string
	// Source performs one pass per call.
	Source Source
	// Interval between passes (default 5 minutes). It is raised to
	// Provider.Capabilities().MinPollInterval when that is larger.
	Interval time.Duration
}

// Key identifies the job in lock keys: provider|tenant|name.
func (j Job) Key() string {
	name := ""
	if j.Provider != nil {
		name = j.Provider.Name()
	}
	return scopeKey(name, j.Tenant, j.Name)
}

func (j Job) interval() time.Duration {
	d := j.Interval
	if d <= 0 {
		d = DefaultInterval
	}
	if j.Provider != nil {
		if m := j.Provider.Capabilities().MinPollInterval; m > d {
			d = m
		}
	}
	return d
}

// Runner schedules jobs, one goroutine each. Configure the exported
// fields before calling Add and Run.
type Runner struct {
	// Sink receives every event. Required.
	Sink Sink
	// Locker, when set, makes each pass hold the lock "watch|"+Job.Key()
	// so only one process polls an account at a time. Passes that find
	// the lock held are skipped silently.
	Locker Locker
	// LockTTL is the lease length (default: twice the job interval, at
	// least one minute).
	LockTTL time.Duration
	// Jitter randomizes each wait by ±Jitter×interval (default 0.1; a
	// negative value disables it).
	Jitter float64
	// MaxBackoff caps the wait after repeated retryable errors (default
	// one hour).
	MaxBackoff time.Duration
	// DrainTimeout is how long a pass in flight may keep running after
	// the Run context is cancelled (default 30s).
	DrainTimeout time.Duration
	// OnError observes failed passes. Retryable errors (provider.IsRetryable,
	// which includes rate limits) back the job off exponentially; other
	// errors keep the normal interval.
	OnError func(job Job, err error)

	mu      sync.Mutex
	jobs    []Job
	running bool
}

// Add registers a job. It must be called before Run.
func (r *Runner) Add(j Job) error {
	if j.Source == nil {
		return errors.New("watch: job without source")
	}
	if j.Provider == nil {
		return errors.New("watch: job without provider")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.running {
		return errors.New("watch: Add after Run")
	}
	for _, o := range r.jobs {
		if o.Key() == j.Key() {
			return fmt.Errorf("watch: duplicate job %s", j.Key())
		}
	}
	r.jobs = append(r.jobs, j)
	return nil
}

func (r *Runner) start() ([]Job, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Sink == nil {
		return nil, errors.New("watch: runner without sink")
	}
	if len(r.jobs) == 0 {
		return nil, errors.New("watch: runner without jobs")
	}
	if r.running {
		return nil, errors.New("watch: runner already running")
	}
	r.running = true
	return append([]Job(nil), r.jobs...), nil
}

func (r *Runner) stop() {
	r.mu.Lock()
	r.running = false
	r.mu.Unlock()
}

// Run polls every job until ctx is cancelled, then waits for passes in
// flight (bounded by DrainTimeout) and returns nil. The first pass of
// each job starts immediately.
func (r *Runner) Run(ctx context.Context) error {
	jobs, err := r.start()
	if err != nil {
		return err
	}
	defer r.stop()
	var wg sync.WaitGroup
	for _, j := range jobs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.loop(ctx, j)
		}()
	}
	wg.Wait()
	return nil
}

// RunOnce performs one pass of every job, in order, and returns the
// joined errors. Jobs whose lock is held elsewhere are skipped.
func (r *Runner) RunOnce(ctx context.Context) error {
	jobs, err := r.start()
	if err != nil {
		return err
	}
	defer r.stop()
	var errs []error
	for _, j := range jobs {
		if err := r.pass(ctx, j); err != nil && !errors.Is(err, ErrLocked) {
			r.report(j, err)
			errs = append(errs, fmt.Errorf("%s: %w", j.Key(), err))
		}
	}
	return errors.Join(errs...)
}

func (r *Runner) loop(ctx context.Context, j Job) {
	interval := j.interval()
	failures := 0
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		err := r.pass(ctx, j)
		wait := interval
		switch {
		case err == nil, errors.Is(err, ErrLocked):
			failures = 0
		case ctx.Err() != nil:
			return
		case provider.IsRetryable(err):
			failures++
			wait = r.backoff(interval, failures)
			r.report(j, err)
		default:
			failures = 0
			r.report(j, err)
		}
		timer.Reset(r.jitter(wait))
	}
}

func (r *Runner) report(j Job, err error) {
	if r.OnError != nil {
		r.OnError(j, err)
	}
}

func (r *Runner) backoff(interval time.Duration, failures int) time.Duration {
	limit := r.MaxBackoff
	if limit <= 0 {
		limit = DefaultMaxBackoff
	}
	if limit < interval {
		limit = interval
	}
	d := interval
	for i := 0; i < failures && d < limit; i++ {
		d *= 2
	}
	return min(d, limit)
}

func (r *Runner) jitter(d time.Duration) time.Duration {
	f := r.Jitter
	if f == 0 {
		f = DefaultJitter
	}
	if f < 0 || d <= 0 {
		return d
	}
	span := int64(float64(d) * f)
	if span <= 0 {
		return d
	}
	return d + time.Duration(rand.Int64N(2*span+1)-span)
}

// pass runs one source pass with an optional lock. The source context
// outlives ctx by DrainTimeout so that a pass in flight can finish its
// page and save its cursor.
func (r *Runner) pass(ctx context.Context, j Job) (err error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	if r.Locker != nil {
		ttl := r.LockTTL
		if ttl <= 0 {
			ttl = max(2*j.interval(), time.Minute)
		}
		unlock, err := r.Locker.Lock(ctx, "watch|"+j.Key(), ttl)
		if err != nil {
			return err
		}
		defer func() {
			uctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancel()
			if uerr := unlock(uctx); uerr != nil && err == nil {
				err = uerr
			}
		}()
	}
	drain := r.DrainTimeout
	if drain <= 0 {
		drain = DefaultDrainTimeout
	}
	passCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		select {
		case <-done:
			return
		case <-ctx.Done():
		}
		t := time.NewTimer(drain)
		defer t.Stop()
		select {
		case <-done:
		case <-t.C:
			cancel()
		}
	}()
	defer func() {
		close(done)
		wg.Wait()
		cancel()
	}()
	return j.Source.Run(passCtx, func(ev Event) error { return r.Sink.Publish(passCtx, ev) })
}
