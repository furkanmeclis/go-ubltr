package main

import (
	"fmt"
	"strconv"

	"github.com/furkanmeclis/go-ubltr/provider"
	"github.com/furkanmeclis/go-ubltr/provider/providertest"
	// Integrator modules register themselves with provider.Register when
	// imported. Add a blank import per integrator module here, e.g.
	//
	//	_ "github.com/furkanmeclis/go-ubltr/provider/<name>"
)

// fakeProvider is the in-memory integrator registered for demos and
// tests. It is not listed in help output.
const fakeProvider = "fake"

func init() {
	provider.Register(fakeProvider, openFake)
}

// openFake opens providertest.Fake. Option seed=N delivers N incoming
// e-Fatura documents with fixed UUIDs, so `ubltr watch --provider fake
// --option seed=2 --once` prints two events.
func openFake(cfg provider.Config) (provider.Provider, error) {
	f := providertest.NewFake()
	if s, ok := cfg.Options["seed"]; ok {
		n, err := strconv.Atoi(s)
		if err != nil || n < 0 {
			return nil, fmt.Errorf("fake: option seed=%q: want a non-negative integer", s)
		}
		for i := range n {
			f.Deliver(provider.DocumentInfo{
				Kind:       provider.EInvoice,
				UUID:       fmt.Sprintf("00000000-0000-4000-8000-%012d", i+1),
				ID:         fmt.Sprintf("FAK2026%09d", i+1),
				Profile:    "TEMELFATURA",
				Type:       "SATIS",
				SenderVKN:  "1234567890",
				SenderName: "Fake Gönderici A.Ş.",
			}, nil)
		}
	}
	return f, nil
}

// publicProviders lists registered integrators without the hidden fake.
func publicProviders() []string {
	var out []string
	for _, n := range provider.Registered() {
		if n != fakeProvider {
			out = append(out, n)
		}
	}
	return out
}
