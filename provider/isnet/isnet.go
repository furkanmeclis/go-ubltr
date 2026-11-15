// Package isnet is a read-only client for the İşNet (NetteFatura) özel
// entegratör SOAP services: InvoiceService for e-Fatura and e-Arşiv, and
// AddressBookService for the registered-user lookup.
//
// İşNet requests carry no user name or password. Access is granted to a
// fixed source IP that İşNet allowlists, together with the company VKN
// sent as CompanyTaxCode in every request. From an IP that is not on the
// list the service does not answer at all, so the failure is a timeout;
// see IsIPNotAllowed.
//
// Request and response shapes follow the WSDL served by the İşNet test
// system (kept under testdata/wsdl) and the İşNet web-service interface
// documents as summarised in the integration notes cited in README.md.
// Only calls that do not change document state are implemented.
//
// Importing the package registers it as "isnet" with provider.Register.
package isnet

import (
	"errors"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/go-ubltr/provider"
	"github.com/furkanmeclis/go-ubltr/provider/soap"
)

// Name is the registry name.
const Name = "isnet"

// Service endpoints. İşNet does not list them in its interface documents;
// they are the addresses used by the community integrations cited in
// README.md and match the https port of the published test WSDL.
const (
	TestInvoiceURL     = "https://einvoiceservicetest.isnet.net.tr/InvoiceService/ServiceContract/InvoiceService.svc"
	LiveInvoiceURL     = "https://einvoiceservice.isnet.net.tr/InvoiceService/ServiceContract/InvoiceService.svc"
	TestAddressBookURL = "https://einvoiceservicetest.isnet.net.tr/AddressBookService/ServiceContract/AddressBookService.svc"
	LiveAddressBookURL = "https://einvoiceservice.isnet.net.tr/AddressBookService/ServiceContract/AddressBookService.svc"
)

// liveHost is the host that filters callers by IP.
const liveHost = "einvoiceservice.isnet.net.tr"

// Defaults.
const (
	DefaultTimeout     = 2 * time.Minute
	DefaultDialTimeout = 15 * time.Second
	DefaultPageSize    = 20
)

// Config configures a Client.
type Config struct {
	// VKN is the company VKN/TCKN sent as CompanyTaxCode. It is the only
	// account identifier İşNet uses; take it from a verified connection
	// record, never from user input, because the allowlisted IP may be
	// authorised for several companies.
	VKN string
	// VendorNumbers are the company's units (CompanyVendorNumber, see
	// Client.Vendors). When set they are sent as
	// IncludeCompanyVendorNumberList in e-Fatura searches.
	VendorNumbers []string
	// Test selects the İşNet test system. The default is the live system,
	// which only answers allowlisted IPs.
	Test bool
	// Endpoint overrides the InvoiceService URL.
	Endpoint string
	// AddressBookEndpoint overrides the AddressBookService URL.
	AddressBookEndpoint string
	// IPFiltered marks the endpoints as IP-filtered, so timeouts are
	// reported as ip-not-allowlisted errors. It is implied when an
	// endpoint is on the live İşNet host.
	IPFiltered bool
	// HTTP overrides the HTTP client. DialTimeout is not applied to it.
	HTTP provider.Doer
	// Timeout bounds each call (default DefaultTimeout; negative disables).
	Timeout time.Duration
	// DialTimeout bounds the TCP connect and TLS handshake of the default
	// HTTP client (default DefaultDialTimeout).
	DialTimeout time.Duration
	// PageSize is the default RecordsPerPage (default 20, as recommended
	// in the integration notes).
	PageSize int
}

// Client talks to İşNet. It is safe for concurrent use.
type Client struct {
	cfg         Config
	invoice     *soap.Client
	addressBook *soap.Client
	// invoiceFiltered / addressBookFiltered are true when the endpoint
	// filters callers by IP.
	invoiceFiltered, addressBookFiltered bool
}

// New returns a Client.
func New(cfg Config) (*Client, error) {
	cfg.VKN = strings.TrimSpace(cfg.VKN)
	if cfg.VKN == "" {
		return nil, errors.New("isnet: VKN is required (sent as CompanyTaxCode)")
	}
	if cfg.Endpoint == "" {
		cfg.Endpoint = LiveInvoiceURL
		if cfg.Test {
			cfg.Endpoint = TestInvoiceURL
		}
	}
	if cfg.AddressBookEndpoint == "" {
		cfg.AddressBookEndpoint = LiveAddressBookURL
		if cfg.Test {
			cfg.AddressBookEndpoint = TestAddressBookURL
		}
	}
	for _, u := range []string{cfg.Endpoint, cfg.AddressBookEndpoint} {
		p, err := url.Parse(u)
		if err != nil || p.Host == "" {
			return nil, errors.New("isnet: invalid endpoint " + strconv.Quote(u))
		}
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = DefaultTimeout
	}
	if cfg.DialTimeout <= 0 {
		cfg.DialTimeout = DefaultDialTimeout
	}
	if cfg.PageSize <= 0 {
		cfg.PageSize = DefaultPageSize
	}
	if cfg.HTTP == nil {
		cfg.HTTP = defaultHTTP(cfg.DialTimeout)
	}
	c := &Client{
		cfg:                 cfg,
		invoice:             &soap.Client{URL: cfg.Endpoint, HTTP: cfg.HTTP, Provider: Name},
		addressBook:         &soap.Client{URL: cfg.AddressBookEndpoint, HTTP: cfg.HTTP, Provider: Name},
		invoiceFiltered:     cfg.IPFiltered || isLiveHost(cfg.Endpoint),
		addressBookFiltered: cfg.IPFiltered || isLiveHost(cfg.AddressBookEndpoint),
	}
	return c, nil
}

func isLiveHost(u string) bool {
	p, err := url.Parse(u)
	return err == nil && strings.EqualFold(p.Hostname(), liveHost)
}

// defaultHTTP bounds connect and TLS handshake, which is where an
// unanswered SYN from a non-allowlisted IP shows up first.
func defaultHTTP(dial time.Duration) *http.Client {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.DialContext = (&net.Dialer{Timeout: dial, KeepAlive: 30 * time.Second}).DialContext
	tr.TLSHandshakeTimeout = dial
	return &http.Client{Transport: tr}
}

func init() {
	provider.Register(Name, func(g provider.Config) (provider.Provider, error) {
		cfg, err := fromGeneric(g)
		if err != nil {
			return nil, err
		}
		return New(cfg)
	})
}

// fromGeneric maps provider.Config. Username, Password and APIKey are not
// used. Options keys: addressbook_endpoint, vendor_numbers (comma
// separated), ip_filtered (bool), timeout and dial_timeout (Go durations),
// page_size.
func fromGeneric(g provider.Config) (Config, error) {
	o := func(k string) string { return strings.TrimSpace(g.Options[k]) }
	cfg := Config{
		VKN:                 g.VKN,
		Test:                g.Test,
		Endpoint:            g.Endpoint,
		AddressBookEndpoint: o("addressbook_endpoint"),
		HTTP:                g.HTTP,
	}
	if s := o("vendor_numbers"); s != "" {
		for _, v := range strings.Split(s, ",") {
			if v = strings.TrimSpace(v); v != "" {
				cfg.VendorNumbers = append(cfg.VendorNumbers, v)
			}
		}
	}
	if s := o("ip_filtered"); s != "" {
		b, err := strconv.ParseBool(s)
		if err != nil {
			return cfg, errors.New("isnet: option ip_filtered: " + err.Error())
		}
		cfg.IPFiltered = b
	}
	for k, dst := range map[string]*time.Duration{"timeout": &cfg.Timeout, "dial_timeout": &cfg.DialTimeout} {
		if s := o(k); s != "" {
			d, err := time.ParseDuration(s)
			if err != nil {
				return cfg, errors.New("isnet: option " + k + ": " + err.Error())
			}
			*dst = d
		}
	}
	if s := o("page_size"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n <= 0 {
			return cfg, errors.New("isnet: option page_size: want a positive integer")
		}
		cfg.PageSize = n
	}
	return cfg, nil
}

// Name implements provider.Provider.
func (c *Client) Name() string { return Name }

// Capabilities implements provider.Provider. Sending, answering, archiving
// and cancelling calls exist in the İşNet WSDL but change document state
// and are deliberately not implemented.
func (c *Client) Capabilities() provider.Capabilities {
	return provider.Capabilities{
		Operations: map[provider.DocumentKind][]provider.Operation{
			provider.EInvoice: {provider.OpStatus, provider.OpListInbox, provider.OpListOutbox, provider.OpFetch, provider.OpCheckUser},
			provider.EArchive: {provider.OpStatus, provider.OpListOutbox, provider.OpFetch},
		},
		Formats:      []provider.Format{provider.FormatUBL, provider.FormatPDF, provider.FormatHTML},
		Protocol:     "soap",
		TestEndpoint: true,
	}
}

var (
	_ provider.Provider      = (*Client)(nil)
	_ provider.StatusChecker = (*Client)(nil)
	_ provider.InboxLister   = (*Client)(nil)
	_ provider.OutboxLister  = (*Client)(nil)
	_ provider.Fetcher       = (*Client)(nil)
	_ provider.UserChecker   = (*Client)(nil)
)
