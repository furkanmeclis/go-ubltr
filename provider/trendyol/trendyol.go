// Package trendyol is a client for the Trendyol E-Faturam REST API
// (https://developers.trendyolefaturam.com). It sends UBL-TR XML with the
// "by-file" endpoints, so callers keep building documents with go-ubltr.
//
// Two account types are supported, as in the documentation:
//
//   - API users (bireysel entegrasyon) sign in with signIn and act for
//     their own company; UserID and CompanyID must be configured.
//   - Marketplace integrators (pazaryeri entegratörü) sign in with signIn
//     and then customerSignIn, which returns the customer token, userId
//     and companyId.
//
// Importing the package registers it as "trendyol" with provider.Register.
package trendyol

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/furkanmeclis/go-ubltr/provider"
)

// Name is the registry name.
const Name = "trendyol"

// Gateway addresses from the documentation ("Ortam Bilgileri").
const (
	TestBaseURL = "https://stage-apigateway.trendyolefaturam.com"
	LiveBaseURL = "https://apigateway.trendyolecozum.com"
)

// Config configures a Client.
type Config struct {
	// Email and Password sign in with POST /api/auth/signin (the API user,
	// or the marketplace integrator account).
	Email, Password string
	// CustomerEmail, CustomerPassword and CustomerTaxID switch to
	// marketplace mode: the client then calls customerSignIn and works for
	// that customer.
	CustomerEmail, CustomerPassword, CustomerTaxID string
	// UserID and CompanyID are required for API users; in marketplace mode
	// they come from customerSignIn.
	UserID, CompanyID int64
	// Source is the "source" field of created documents (default PARTNER).
	Source string
	// InvoicePrefix is sent as "prefix" with autoInvoiceId when a document
	// has no cbc:ID.
	InvoicePrefix string
	// Test selects the stage gateway; BaseURL overrides both.
	Test    bool
	BaseURL string
	// HTTP overrides the HTTP client.
	HTTP provider.Doer
}

// Client talks to Trendyol E-Faturam. It is safe for concurrent use.
type Client struct {
	cfg  Config
	base string
	http provider.Doer

	mu        sync.Mutex
	token     string
	userID    int64
	companyID int64
}

func (c Config) marketplace() bool { return c.CustomerEmail != "" }

// New returns a Client. Sign-in happens on the first call.
func New(cfg Config) (*Client, error) {
	if cfg.Email == "" || cfg.Password == "" {
		return nil, errors.New("trendyol: Email and Password are required")
	}
	if cfg.marketplace() {
		if cfg.CustomerPassword == "" || cfg.CustomerTaxID == "" {
			return nil, errors.New("trendyol: CustomerPassword and CustomerTaxID are required with CustomerEmail")
		}
	} else if cfg.UserID == 0 || cfg.CompanyID == 0 {
		return nil, errors.New("trendyol: UserID and CompanyID are required for API users")
	}
	base := cfg.BaseURL
	if base == "" {
		base = LiveBaseURL
		if cfg.Test {
			base = TestBaseURL
		}
	}
	if cfg.Source == "" {
		cfg.Source = "PARTNER"
	}
	doer := cfg.HTTP
	if doer == nil {
		doer = http.DefaultClient
	}
	return &Client{cfg: cfg, base: strings.TrimRight(base, "/"), http: doer, userID: cfg.UserID, companyID: cfg.CompanyID}, nil
}

func init() {
	provider.Register(Name, func(g provider.Config) (provider.Provider, error) {
		return New(fromGeneric(g))
	})
}

// fromGeneric maps provider.Config: Username / Password are the sign-in
// e-mail and password, Endpoint is the base URL. Options keys: user_id,
// company_id, customer_email, customer_password, customer_tax_id (defaults
// to VKN), source, invoice_prefix.
func fromGeneric(g provider.Config) Config {
	o := func(k string) string { return strings.TrimSpace(g.Options[k]) }
	n := func(k string) int64 { v, _ := strconv.ParseInt(o(k), 10, 64); return v }
	cfg := Config{
		Email:            g.Username,
		Password:         g.Password,
		CustomerEmail:    o("customer_email"),
		CustomerPassword: o("customer_password"),
		CustomerTaxID:    o("customer_tax_id"),
		UserID:           n("user_id"),
		CompanyID:        n("company_id"),
		Source:           o("source"),
		InvoicePrefix:    o("invoice_prefix"),
		Test:             g.Test,
		BaseURL:          g.Endpoint,
		HTTP:             g.HTTP,
	}
	if cfg.CustomerEmail != "" && cfg.CustomerTaxID == "" {
		cfg.CustomerTaxID = g.VKN
	}
	return cfg
}

// Name implements provider.Provider.
func (c *Client) Name() string { return Name }

// Capabilities implements provider.Provider.
func (c *Client) Capabilities() provider.Capabilities {
	return provider.Capabilities{
		Operations: map[provider.DocumentKind][]provider.Operation{
			provider.EInvoice: {
				provider.OpSend, provider.OpStatus, provider.OpListInbox, provider.OpFetch,
				provider.OpRespond, provider.OpCheckUser,
			},
			provider.EArchive: {
				provider.OpSend, provider.OpStatus, provider.OpFetch, provider.OpCancelEArchive,
			},
		},
		Formats:        []provider.Format{provider.FormatUBL, provider.FormatPDF, provider.FormatHTML},
		Protocol:       "rest",
		AssignsNumbers: true,
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
	_ provider.Fetcher           = (*Client)(nil)
	_ provider.Responder         = (*Client)(nil)
	_ provider.UserChecker       = (*Client)(nil)
	_ provider.EArchiveCanceller = (*Client)(nil)
)
