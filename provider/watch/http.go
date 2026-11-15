package watch

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/go-ubltr/provider"
)

// Headers set by [HTTPSink].
const (
	HeaderSignature = "X-Ubltr-Signature"
	HeaderTimestamp = "X-Ubltr-Timestamp"
	HeaderEventID   = "X-Ubltr-Event-Id"
	HeaderEventType = "X-Ubltr-Event-Type"
)

// HTTPSink POSTs each event as JSON to URL, turning polling back into the
// webhook a receiving service expects.
//
// When Secret is set, the request carries
//
//	X-Ubltr-Timestamp: <unix seconds>
//	X-Ubltr-Signature: sha256=<hex HMAC-SHA256 of "<timestamp>.<body>">
//
// which the receiver checks with [VerifySignature]. Any 2xx response is
// success. 429, 408 and 5xx responses and network errors are retried with
// provider.Retry; other statuses fail at once.
type HTTPSink struct {
	URL    string
	Secret []byte
	// Client defaults to an *http.Client with a 30 second timeout.
	Client provider.Doer
	// Retry defaults to provider.Retry's defaults (3 attempts).
	Retry provider.RetryPolicy
	// Header is added to every request.
	Header http.Header
	// Now replaces time.Now (tests).
	Now func() time.Time
}

var defaultHTTPClient = &http.Client{Timeout: 30 * time.Second}

// Publish implements Sink.
func (s *HTTPSink) Publish(ctx context.Context, ev Event) error {
	if s.URL == "" {
		return errors.New("watch: HTTPSink without URL")
	}
	body, err := json.Marshal(ev)
	if err != nil {
		return fmt.Errorf("watch: encode event: %w", err)
	}
	client := s.Client
	if client == nil {
		client = defaultHTTPClient
	}
	now := time.Now
	if s.Now != nil {
		now = s.Now
	}
	return provider.Retry(ctx, s.Retry, func(ctx context.Context) error {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.URL, bytes.NewReader(body))
		if err != nil {
			return err
		}
		for k, vs := range s.Header {
			for _, v := range vs {
				req.Header.Add(k, v)
			}
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "go-ubltr-watch")
		req.Header.Set(HeaderEventID, ev.ID)
		req.Header.Set(HeaderEventType, string(ev.Type))
		if len(s.Secret) > 0 {
			ts := strconv.FormatInt(now().Unix(), 10)
			req.Header.Set(HeaderTimestamp, ts)
			req.Header.Set(HeaderSignature, Sign(s.Secret, ts, body))
		}
		resp, err := client.Do(req)
		if err != nil {
			return provider.TransportError("watch-http", "publish", err)
		}
		defer func() { _ = resp.Body.Close() }()
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return nil
		}
		return provider.HTTPError("watch-http", "publish", resp.StatusCode, data)
	})
}

// Sign returns the X-Ubltr-Signature value for a timestamp and body.
func Sign(secret []byte, timestamp string, body []byte) string {
	m := hmac.New(sha256.New, secret)
	m.Write([]byte(timestamp))
	m.Write([]byte{'.'})
	m.Write(body)
	return "sha256=" + hex.EncodeToString(m.Sum(nil))
}

// ErrBadSignature is returned by [VerifySignature].
var ErrBadSignature = errors.New("watch: bad signature")

// VerifySignature checks the headers of a request sent by [HTTPSink].
// tolerance bounds the age of the timestamp (0 skips the check); now is
// usually time.Now().
func VerifySignature(secret []byte, signature, timestamp string, body []byte, tolerance time.Duration, now time.Time) error {
	if !strings.HasPrefix(signature, "sha256=") {
		return ErrBadSignature
	}
	if tolerance > 0 {
		sec, err := strconv.ParseInt(timestamp, 10, 64)
		if err != nil {
			return ErrBadSignature
		}
		age := now.Sub(time.Unix(sec, 0))
		if age > tolerance || age < -tolerance {
			return fmt.Errorf("%w: timestamp outside tolerance", ErrBadSignature)
		}
	}
	if !hmac.Equal([]byte(Sign(secret, timestamp, body)), []byte(signature)) {
		return ErrBadSignature
	}
	return nil
}

var _ Sink = (*HTTPSink)(nil)
