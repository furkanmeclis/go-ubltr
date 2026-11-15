// Package qnb is a client for the QNB eSolutions (eFinans) özel entegratör
// SOAP services: the e-Fatura connectorService and the e-Arşiv
// EarsivWebService.
//
// Request and response shapes follow the WSDLs published on the QNB test
// systems and the QNB eSolutions API documentation
// (https://www.qnbesolutions.com.tr/api-entegrasyon.html). Authentication
// uses a WS-Security UsernameToken header on every request, which is one of
// the two methods the documentation describes.
//
// Importing the package registers it as "qnb" with provider.Register.
package qnb

import (
	"errors"
	"strings"

	"github.com/furkanmeclis/go-ubltr/provider"
	"github.com/furkanmeclis/go-ubltr/provider/soap"
)

// Name is the registry name.
const Name = "qnb"

// Test endpoints from the QNB documentation. Live endpoints are shared by
// QNB after the test phase and must be set in Config.
const (
	TestConnectorURL = "https://erpefaturatest1.qnbesolutions.com.tr/efatura/ws/connectorService"
	TestEArchiveURL  = "https://earsivtest.qnbesolutions.com.tr/earsiv/ws/EarsivWebService"
)

const (
	nsConnector = "http://service.connector.uut.cs.com.tr/"
	nsEArchive  = "http://service.earsiv.uut.cs.com.tr/"
)

// Config configures a Client.
type Config struct {
	// Username and Password are the web-service user of the e-Fatura
	// connector service.
	Username, Password string
	// EArchiveUsername and EArchivePassword are the e-Arşiv web-service
	// user. QNB usually issues a separate e-Arşiv user; when empty the
	// e-Fatura credentials are used.
	EArchiveUsername, EArchivePassword string
	// VKN is the account's VKN/TCKN (vergiTcKimlikNo / vkn).
	VKN string
	// ERPCode is the erpKodu given by the QNB project team.
	ERPCode string
	// Test selects the QNB test endpoints when the URLs below are empty.
	Test bool
	// ConnectorURL and EArchiveURL override the endpoints.
	ConnectorURL, EArchiveURL string
	// Branch and CashRegister are the e-Arşiv sube / kasa codes
	// (default "DFLT").
	Branch, CashRegister string
	// InvoiceSeries is sent as seriNo (e-Fatura) or faturaSeri (e-Arşiv)
	// when a document has no cbc:ID, so the portal assigns the number.
	InvoiceSeries string
	// HTTP overrides the HTTP client.
	HTTP provider.Doer
}

// Client talks to QNB eSolutions. It is safe for concurrent use.
type Client struct {
	cfg       Config
	connector *soap.Client
	earchive  *soap.Client
}

// New returns a Client. Live use needs ConnectorURL (and EArchiveURL for
// e-Arşiv) because QNB does not publish live endpoints.
func New(cfg Config) (*Client, error) {
	if cfg.Username == "" || cfg.Password == "" {
		return nil, errors.New("qnb: Username and Password are required")
	}
	if cfg.VKN == "" {
		return nil, errors.New("qnb: VKN is required")
	}
	if cfg.ConnectorURL == "" && cfg.Test {
		cfg.ConnectorURL = TestConnectorURL
	}
	if cfg.EArchiveURL == "" && cfg.Test {
		cfg.EArchiveURL = TestEArchiveURL
	}
	if cfg.ConnectorURL == "" && cfg.EArchiveURL == "" {
		return nil, errors.New("qnb: set ConnectorURL / EArchiveURL (live endpoints are given by QNB) or Test")
	}
	if cfg.EArchiveUsername == "" {
		cfg.EArchiveUsername, cfg.EArchivePassword = cfg.Username, cfg.Password
	}
	if cfg.Branch == "" {
		cfg.Branch = "DFLT"
	}
	if cfg.CashRegister == "" {
		cfg.CashRegister = "DFLT"
	}
	c := &Client{cfg: cfg}
	if cfg.ConnectorURL != "" {
		c.connector = &soap.Client{URL: cfg.ConnectorURL, HTTP: cfg.HTTP, Provider: Name, Header: soap.NewUsernameToken(cfg.Username, cfg.Password)}
	}
	if cfg.EArchiveURL != "" {
		c.earchive = &soap.Client{URL: cfg.EArchiveURL, HTTP: cfg.HTTP, Provider: Name, Header: soap.NewUsernameToken(cfg.EArchiveUsername, cfg.EArchivePassword)}
	}
	return c, nil
}

func init() {
	provider.Register(Name, func(g provider.Config) (provider.Provider, error) {
		return New(fromGeneric(g))
	})
}

// fromGeneric maps provider.Config. Options keys: earchive_endpoint,
// earchive_username, earchive_password, erp_code, branch, cash_register,
// invoice_series.
func fromGeneric(g provider.Config) Config {
	o := func(k string) string { return strings.TrimSpace(g.Options[k]) }
	return Config{
		Username:         g.Username,
		Password:         g.Password,
		VKN:              g.VKN,
		Test:             g.Test,
		ConnectorURL:     g.Endpoint,
		EArchiveURL:      o("earchive_endpoint"),
		EArchiveUsername: o("earchive_username"),
		EArchivePassword: o("earchive_password"),
		ERPCode:          o("erp_code"),
		Branch:           o("branch"),
		CashRegister:     o("cash_register"),
		InvoiceSeries:    o("invoice_series"),
		HTTP:             g.HTTP,
	}
}

// Name implements provider.Provider.
func (c *Client) Name() string { return Name }

// Capabilities implements provider.Provider. Only operations whose request
// and response shapes are confirmed by the published WSDLs and the QNB
// documentation are declared.
func (c *Client) Capabilities() provider.Capabilities {
	ops := map[provider.DocumentKind][]provider.Operation{}
	if c.connector != nil {
		ops[provider.EInvoice] = []provider.Operation{
			provider.OpSend, provider.OpStatus, provider.OpListInbox, provider.OpFetch,
			provider.OpMarkTaken, provider.OpCheckUser, provider.OpCredits,
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
	_ provider.Fetcher           = (*Client)(nil)
	_ provider.Acknowledger      = (*Client)(nil)
	_ provider.UserChecker       = (*Client)(nil)
	_ provider.CreditChecker     = (*Client)(nil)
	_ provider.EArchiveCanceller = (*Client)(nil)
)
