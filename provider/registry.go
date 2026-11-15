package provider

import (
	"fmt"
	"sort"
	"sync"
)

// Config is the generic configuration used by the registry. Integrator
// modules also offer typed constructors; Config exists so tools such as
// the ubltr CLI can open any registered integrator by name.
type Config struct {
	// Username / Password are web-service credentials.
	Username, Password string
	// APIKey is used by REST integrators.
	APIKey string
	// Endpoint overrides the base URL (test, live or a compatible vendor).
	Endpoint string
	// Test selects the integrator's test system when Endpoint is empty.
	Test bool
	// VKN is the account's VKN/TCKN when the API needs it.
	VKN string
	// Options holds integrator-specific settings.
	Options map[string]string
	// HTTP overrides the HTTP client.
	HTTP Doer
}

// Factory opens an integrator from Config.
type Factory func(Config) (Provider, error)

var (
	regMu    sync.RWMutex
	registry = map[string]Factory{}
)

// Register makes a factory available by name. Integrator modules call it
// from init; importing the module (even with _) registers it.
func Register(name string, f Factory) {
	regMu.Lock()
	defer regMu.Unlock()
	if f == nil {
		panic("provider: Register nil factory for " + name)
	}
	if _, dup := registry[name]; dup {
		panic("provider: Register called twice for " + name)
	}
	registry[name] = f
}

// Open opens a registered integrator.
func Open(name string, cfg Config) (Provider, error) {
	regMu.RLock()
	f, ok := registry[name]
	regMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("provider: unknown integrator %q (forgotten import?)", name)
	}
	return f(cfg)
}

// Registered lists registered integrator names, sorted.
func Registered() []string {
	regMu.RLock()
	defer regMu.RUnlock()
	out := make([]string, 0, len(registry))
	for n := range registry {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}
