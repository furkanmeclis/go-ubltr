package providertest

import (
	"testing"

	"github.com/furkanmeclis/go-ubltr/provider"
)

// CheckCapabilities fails t when p declares an operation it does not
// implement, or declares nothing.
func CheckCapabilities(t testing.TB, p provider.Provider) {
	t.Helper()
	if p.Name() == "" {
		t.Error("Name() is empty")
	}
	c := p.Capabilities()
	if len(c.Kinds()) == 0 {
		t.Error("Capabilities declares no document kind")
	}
	for kind, ops := range c.Operations {
		seen := map[provider.Operation]bool{}
		for _, op := range ops {
			if seen[op] {
				t.Errorf("%s: operation %s declared twice", kind, op)
			}
			seen[op] = true
			if !provider.Supports(p, kind, op) {
				t.Errorf("%s: declares %s but does not implement its interface", kind, op)
			}
		}
	}
	switch c.Protocol {
	case "soap", "rest", "soap+rest", "memory":
	default:
		t.Errorf("Protocol %q: want soap, rest or soap+rest", c.Protocol)
	}
}

// CheckCodeTable fails t when a table is empty or has entries without a
// message.
func CheckCodeTable(t testing.TB, table *provider.CodeTable) {
	t.Helper()
	if table.Len() == 0 {
		t.Fatalf("code table %s is empty", table.Name())
	}
	for _, c := range table.Codes() {
		if c.Code == "" {
			t.Errorf("%s: empty code", table.Name())
		}
		if c.Message == "" {
			t.Errorf("%s: code %s has no message", table.Name(), c.Code)
		}
	}
}
