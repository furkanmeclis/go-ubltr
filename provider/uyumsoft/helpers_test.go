package uyumsoft

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeService replays fixture envelopes from testdata/fixtures/<vendor>/.
// The fixture is chosen by the operation in SOAPAction; set override to
// use another file (e.g. "SendInvoice_failed") or status for faults.
type fakeService struct {
	t        *testing.T
	vendor   string
	mu       sync.Mutex
	override map[string]string
	status   map[string]int
	bodies   map[string]string
	actions  []string
	headers  []http.Header
}

func newService(t *testing.T, vendor string) (*fakeService, *httptest.Server) {
	t.Helper()
	f := &fakeService{t: t, vendor: vendor, override: map[string]string{}, status: map[string]int{}, bodies: map[string]string{}}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	return f, srv
}

func (f *fakeService) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	action := strings.Trim(r.Header.Get("SOAPAction"), `"`)
	op := action[strings.LastIndexByte(action, '/')+1:]
	f.mu.Lock()
	f.actions = append(f.actions, action)
	f.headers = append(f.headers, r.Header.Clone())
	f.bodies[op] = string(body)
	name, code := op, http.StatusOK
	if o, ok := f.override[op]; ok {
		name = o
	}
	if s, ok := f.status[op]; ok {
		code = s
	}
	f.mu.Unlock()
	data, err := os.ReadFile(filepath.Join("testdata", "fixtures", f.vendor, name+".xml"))
	if err != nil {
		f.t.Errorf("no fixture for %s: %v", name, err)
		http.Error(w, "no fixture", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/xml; charset=utf-8")
	w.WriteHeader(code)
	_, _ = w.Write(data)
}

func (f *fakeService) body(op string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.bodies[op]
}

var fixedNow = time.Date(2026, 11, 15, 9, 0, 0, 0, time.UTC)

func uyumsoftClient(t *testing.T) (*Client, *fakeService) {
	t.Helper()
	f, srv := newService(t, "uyumsoft")
	c, err := New(Config{Username: "user", Password: "pass", Endpoint: srv.URL, HTTP: srv.Client(), Now: func() time.Time { return fixedNow }})
	if err != nil {
		t.Fatal(err)
	}
	return c, f
}

func miClient(t *testing.T) (*Client, *fakeService) {
	t.Helper()
	f, srv := newService(t, "muhasebeizleme")
	c, err := New(Config{Username: "entegrasyon", Password: "api-key", Preset: Muhasebeizleme, Endpoint: srv.URL, HTTP: srv.Client(), Now: func() time.Time { return fixedNow }})
	if err != nil {
		t.Fatal(err)
	}
	return c, f
}

func readFile(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func mustContain(t *testing.T, s string, subs ...string) {
	t.Helper()
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			t.Errorf("missing %q in:\n%s", sub, s)
		}
	}
}
