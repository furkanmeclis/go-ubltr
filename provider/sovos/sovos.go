// Package sovos is a client for the Sovos Türkiye (formerly FIT / Foriba)
// Bulut özel entegratör SOAP services: ClientEInvoiceServices (e-Fatura)
// and ClientEArsivServices (e-Arşiv).
//
// Shapes follow ClientEInvoiceServices-2.2.wsdl and
// EArchiveInvoiceService_v2.wsdl with the SOAP samples and code lists
// from the Sovos API packages published at https://api.fitbulut.com/servis/.
// The services use HTTP Basic authentication and SOAP 1.1 only.
//
// Digital Planet, which Sovos also owns, runs a different API and is not
// covered here.
//
// Importing the package registers it as "sovos" with provider.Register.
package sovos

import (
	"errors"
	"net/http"
	"strings"

	"github.com/furkanmeclis/go-ubltr/provider"
	"github.com/furkanmeclis/go-ubltr/provider/soap"
)

// Name is the registry name.
const Name = "sovos"

// Test endpoints from the Sovos sample client configuration. Live
// endpoints are given by Sovos with the account and must be set in Config.
const (
	TestEInvoiceURL = "https://efaturawstest.fitbulut.com/ClientEInvoiceServices/ClientEInvoiceServicesPort.svc"
	TestEArchiveURL = "https://earsivwstest.fitbulut.com/ClientEArsivServicesPort.svc"
)

// Config configures a Client.
type Config struct {
	// Username and Password are the web-service user (HTTP Basic).
	Username, Password string
	// EArchiveUsername and EArchivePassword default to the above.
	EArchiveUsername, EArchivePassword string
	// VKN is the account's VKN/TCKN.
	VKN string
	// SenderAlias is the GB etiketi used to send and to query outgoing
	// e-Fatura documents (Identifier / SenderIdentifier).
	SenderAlias string
	// PostboxAlias is the PK etiketi used to list and download incoming
	// e-Fatura documents.
	PostboxAlias string
	// Branch is the e-Arşiv şube (customizationParams BRANCH); empty uses
	// the account's default branch.
	Branch string
	// Test selects the Sovos test endpoints when the URLs are empty.
	Test bool
	// EInvoiceURL and EArchiveURL override the endpoints.
	EInvoiceURL, EArchiveURL string
	// HTTP overrides the HTTP client.
	HTTP provider.Doer
}

// Client talks to Sovos Bulut. It is safe for concurrent use.
type Client struct {
	cfg      Config
	einvoice *soap.Client
	earchive *soap.Client
}

// New returns a Client.
func New(cfg Config) (*Client, error) {
	if cfg.Username == "" || cfg.Password == "" {
		return nil, errors.New("sovos: Username and Password are required")
	}
	if cfg.VKN == "" {
		return nil, errors.New("sovos: VKN is required")
	}
	if cfg.Test {
		if cfg.EInvoiceURL == "" {
			cfg.EInvoiceURL = TestEInvoiceURL
		}
		if cfg.EArchiveURL == "" {
			cfg.EArchiveURL = TestEArchiveURL
		}
	}
	if cfg.EInvoiceURL == "" && cfg.EArchiveURL == "" {
		return nil, errors.New("sovos: set EInvoiceURL / EArchiveURL (given by Sovos) or Test")
	}
	if cfg.EArchiveUsername == "" {
		cfg.EArchiveUsername, cfg.EArchivePassword = cfg.Username, cfg.Password
	}
	c := &Client{cfg: cfg}
	if cfg.EInvoiceURL != "" {
		u, p := cfg.Username, cfg.Password
		c.einvoice = &soap.Client{URL: cfg.EInvoiceURL, HTTP: cfg.HTTP, Provider: Name, Prepare: func(r *http.Request) { r.SetBasicAuth(u, p) }}
	}
	if cfg.EArchiveURL != "" {
		u, p := cfg.EArchiveUsername, cfg.EArchivePassword
		c.earchive = &soap.Client{URL: cfg.EArchiveURL, HTTP: cfg.HTTP, Provider: Name, Prepare: func(r *http.Request) { r.SetBasicAuth(u, p) }}
	}
	return c, nil
}

func init() {
	provider.Register(Name, func(g provider.Config) (provider.Provider, error) {
		return New(fromGeneric(g))
	})
}

// fromGeneric maps provider.Config. Options keys: sender_alias,
// postbox_alias, branch, earchive_endpoint, earchive_username,
// earchive_password. provider.Config.Endpoint is the e-Fatura URL.
func fromGeneric(g provider.Config) Config {
	o := func(k string) string { return strings.TrimSpace(g.Options[k]) }
	return Config{
		Username:         g.Username,
		Password:         g.Password,
		VKN:              g.VKN,
		Test:             g.Test,
		EInvoiceURL:      g.Endpoint,
		EArchiveURL:      o("earchive_endpoint"),
		EArchiveUsername: o("earchive_username"),
		EArchivePassword: o("earchive_password"),
		SenderAlias:      o("sender_alias"),
		PostboxAlias:     o("postbox_alias"),
		Branch:           o("branch"),
		HTTP:             g.HTTP,
	}
}

// Name implements provider.Provider.
func (c *Client) Name() string { return Name }

// Capabilities implements provider.Provider.
func (c *Client) Capabilities() provider.Capabilities {
	ops := map[provider.DocumentKind][]provider.Operation{}
	if c.einvoice != nil {
		ops[provider.EInvoice] = []provider.Operation{
			provider.OpSend, provider.OpStatus, provider.OpListInbox, provider.OpListOutbox, provider.OpFetch,
		}
	}
	if c.earchive != nil {
		ops[provider.EArchive] = []provider.Operation{
			provider.OpSend, provider.OpStatus, provider.OpFetch, provider.OpCancelEArchive,
		}
	}
	return provider.Capabilities{
		Operations:     ops,
		Formats:        []provider.Format{provider.FormatUBL, provider.FormatPDF, provider.FormatHTML},
		Protocol:       "soap",
		SignsDocuments: true,
		AcceptsRawUBL:  true,
		MaxBatch:       1,
		TestEndpoint:   true,
	}
}

var (
	_ provider.Provider          = (*Client)(nil)
	_ provider.Sender            = (*Client)(nil)
	_ provider.StatusChecker     = (*Client)(nil)
	_ provider.InboxLister       = (*Client)(nil)
	_ provider.OutboxLister      = (*Client)(nil)
	_ provider.Fetcher           = (*Client)(nil)
	_ provider.EArchiveCanceller = (*Client)(nil)
)
