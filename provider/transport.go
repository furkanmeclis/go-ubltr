package provider

import (
	"context"
	"errors"
	"math/rand/v2"
	"net/http"
	"time"
)

// Doer is the subset of *http.Client used by integrator modules, so callers
// can inject logging, tracing or test transports.
type Doer interface {
	Do(*http.Request) (*http.Response, error)
}

// RetryPolicy configures Retry.
type RetryPolicy struct {
	// Attempts is the total number of tries (default 3).
	Attempts int
	// Base is the first backoff (default 500ms); it doubles each try.
	Base time.Duration
	// Max caps a single backoff (default 30s).
	Max time.Duration
}

// Retry runs fn until it succeeds, returns a non-retryable error (see
// IsRetryable), ctx ends, or attempts run out. Backoff is exponential with
// full jitter.
func Retry(ctx context.Context, p RetryPolicy, fn func(context.Context) error) error {
	if p.Attempts <= 0 {
		p.Attempts = 3
	}
	if p.Base <= 0 {
		p.Base = 500 * time.Millisecond
	}
	if p.Max <= 0 {
		p.Max = 30 * time.Second
	}
	var err error
	for i := 0; i < p.Attempts; i++ {
		if err = fn(ctx); err == nil || !IsRetryable(err) {
			return err
		}
		if i == p.Attempts-1 {
			break
		}
		d := p.Base << i
		if d > p.Max || d <= 0 {
			d = p.Max
		}
		d = time.Duration(rand.Int64N(int64(d) + 1))
		t := time.NewTimer(d)
		select {
		case <-ctx.Done():
			t.Stop()
			return errors.Join(err, ctx.Err())
		case <-t.C:
		}
	}
	return err
}

// HTTPError classifies an HTTP status into an *Error.
func HTTPError(providerName, op string, status int, body []byte) *Error {
	e := &Error{Provider: providerName, Op: op, HTTPStatus: status, Raw: body, Kind: KindTransport}
	switch {
	case status == http.StatusUnauthorized:
		e.Kind = KindAuth
	case status == http.StatusForbidden:
		e.Kind = KindPermission
	case status == http.StatusNotFound:
		e.Kind = KindNotFound
	case status == http.StatusConflict:
		e.Kind = KindDuplicate
	case status == http.StatusTooManyRequests:
		e.Kind = KindRateLimit
		e.Retryable = true
	case status == http.StatusRequestTimeout || status == http.StatusGatewayTimeout:
		e.Kind = KindTimeout
		e.Retryable = true
	case status == http.StatusBadRequest || status == http.StatusUnprocessableEntity:
		e.Kind = KindInvalidRequest
	case status >= 500:
		e.Kind = KindUnavailable
		e.Retryable = true
	}
	e.Message = http.StatusText(status)
	return e
}

// TransportError wraps a network error, marking deadline errors as timeouts.
func TransportError(providerName, op string, err error) *Error {
	e := &Error{Kind: KindTransport, Provider: providerName, Op: op, Err: err, Retryable: true}
	if errors.Is(err, context.DeadlineExceeded) {
		e.Kind = KindTimeout
	}
	if errors.Is(err, context.Canceled) {
		e.Retryable = false
	}
	return e
}
