package watch

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"sync"
)

// ExecSink runs a command for every event, with the event JSON (one line)
// on stdin. A zero exit status is success; anything else fails the event,
// so the source retries it. The environment also carries UBLTR_EVENT_ID,
// UBLTR_EVENT_TYPE, UBLTR_PROVIDER, UBLTR_TENANT, UBLTR_KIND,
// UBLTR_DIRECTION, UBLTR_UUID, UBLTR_STATUS and UBLTR_GIB_CODE for scripts
// that do not want to parse JSON.
//
// Events are handled one at a time, in order.
type ExecSink struct {
	// Path and Args are passed to exec.CommandContext. Use
	// []string{"-c", script} with Path "sh" for shell snippets.
	Path string
	Args []string
	// Dir is the working directory (default: current).
	Dir string
	// Env is appended to the current environment.
	Env []string
	// Stdout and Stderr receive the command's output (default os.Stdout
	// and os.Stderr).
	Stdout, Stderr io.Writer

	mu sync.Mutex
}

// Publish implements Sink.
func (s *ExecSink) Publish(ctx context.Context, ev Event) error {
	if s.Path == "" {
		return errors.New("watch: ExecSink without command")
	}
	body, err := json.Marshal(ev)
	if err != nil {
		return fmt.Errorf("watch: encode event: %w", err)
	}
	cmd := exec.CommandContext(ctx, s.Path, s.Args...)
	cmd.Dir = s.Dir
	cmd.Stdin = bytes.NewReader(append(body, '\n'))
	cmd.Stdout, cmd.Stderr = s.Stdout, s.Stderr
	if cmd.Stdout == nil {
		cmd.Stdout = os.Stdout
	}
	if cmd.Stderr == nil {
		cmd.Stderr = os.Stderr
	}
	cmd.Env = append(os.Environ(),
		"UBLTR_EVENT_ID="+ev.ID,
		"UBLTR_EVENT_TYPE="+string(ev.Type),
		"UBLTR_PROVIDER="+ev.Provider,
		"UBLTR_TENANT="+ev.Tenant,
		"UBLTR_KIND="+string(ev.Kind),
		"UBLTR_DIRECTION="+string(ev.Direction),
		"UBLTR_UUID="+ev.UUID,
		"UBLTR_STATUS="+string(ev.Status),
		"UBLTR_GIB_CODE="+strconv.Itoa(ev.GIBCode),
	)
	cmd.Env = append(cmd.Env, s.Env...)
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("watch: exec %s for %s: %w", s.Path, ev.ID, err)
	}
	return nil
}

var _ Sink = (*ExecSink)(nil)
