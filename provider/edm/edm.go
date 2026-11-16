package edm

import (
	"errors"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/furkanmeclis/go-ubltr/provider"
	"github.com/furkanmeclis/go-ubltr/provider/soap"
)

// Name is the registry name.
const Name = "edm"

// Endpoints with a public WSDL. TestEndpoint is the test address used in
// the rebasesoftware/edm-efatura documentation; the other two are the
// public EFaturaEDM.svc services. Which production host an account uses is
// part of the EDM contract, so production requires Config.Endpoint.
const (
	TestEndpoint       = "https://test.edmbilisim.com.tr/EFaturaEDM21ea/EFaturaEDM.svc"
	PortalEndpoint     = "https://portal2.edmbilisim.com.tr/EFaturaEDM/EFaturaEDM.svc"
	InteraktifEndpoint = "https://interaktif.edmbilisim.com.tr/EFaturaEDM/EFaturaEDM.svc"
)

// Config configures a Provider.
type Config struct {
	// Username and Password are the EDM web-service credentials.
	Username, Password string
	// Endpoint is the full EFaturaEDM.svc URL.
	Endpoint string
	// Test selects TestEndpoint when Endpoint is empty.
	Test bool
	// VKN is the sender VKN/TCKN sent as SENDER/@vkn.
	VKN string
	// ApplicationName, ChannelName, Hostname and Reason fill the matching
	// REQUEST_HEADER fields. ApplicationName and ChannelName default to
	// "go-ubltr" and Reason to "go-ubltr" (EDM requires CHANNEL_NAME and
	// REASON), Hostname to os.Hostname().
	ApplicationName, ChannelName, Hostname, Reason string
	// HTTP overrides the HTTP client.
	HTTP provider.Doer
}

// Provider is the EDM Bilişim integrator. It is safe for concurrent use;
// one session is shared and renewed when EDM reports it as invalid.
type Provider struct {
	cfg    Config
	client *soap.Client
	codes  *provider.CodeTable
	now    func() time.Time

	mu      sync.Mutex
	session string
	gen     uint64
}

var (
	_ provider.Provider      = (*Provider)(nil)
	_ provider.Sender        = (*Provider)(nil)
	_ provider.StatusChecker = (*Provider)(nil)
	_ provider.InboxLister   = (*Provider)(nil)
	_ provider.OutboxLister  = (*Provider)(nil)
	_ provider.Fetcher       = (*Provider)(nil)
	_ provider.UserChecker   = (*Provider)(nil)
	_ provider.Acknowledger  = (*Provider)(nil)
)

// New returns a Provider.
func New(cfg Config) (*Provider, error) {
	if cfg.Username == "" || cfg.Password == "" {
		return nil, errors.New("edm: username and password are required")
	}
	if cfg.Endpoint == "" {
		if !cfg.Test {
			return nil, errors.New("edm: Endpoint is required outside the test system")
		}
		cfg.Endpoint = TestEndpoint
	}
	if cfg.ApplicationName == "" {
		cfg.ApplicationName = "go-ubltr"
	}
	// EDM rejects requests without CHANNEL_NAME ("CHANNEL_NAME verilmeli").
	if cfg.ChannelName == "" {
		cfg.ChannelName = "go-ubltr"
	}
	// EDM also rejects requests without REASON ("REASON verilmeli").
	if cfg.Reason == "" {
		cfg.Reason = "go-ubltr"
	}
	if cfg.Hostname == "" {
		cfg.Hostname, _ = os.Hostname()
		if cfg.Hostname == "" {
			cfg.Hostname = "go-ubltr"
		}
	}
	doer := cfg.HTTP
	if doer == nil {
		doer = &http.Client{Timeout: 2 * time.Minute}
	}
	return &Provider{
		cfg:    cfg,
		client: &soap.Client{URL: cfg.Endpoint, HTTP: doer, Provider: Name},
		codes:  Codes,
		now:    time.Now,
	}, nil
}

// Open is the registry factory. Options: "application_name",
// "channel_name", "hostname", "reason".
func Open(c provider.Config) (provider.Provider, error) {
	o := c.Options
	return New(Config{
		Username:        c.Username,
		Password:        c.Password,
		Endpoint:        c.Endpoint,
		Test:            c.Test,
		VKN:             c.VKN,
		ApplicationName: o["application_name"],
		ChannelName:     o["channel_name"],
		Hostname:        o["hostname"],
		Reason:          o["reason"],
		HTTP:            c.HTTP,
	})
}

func init() { provider.Register(Name, Open) }

// Name implements provider.Provider.
func (p *Provider) Name() string { return Name }

// Capabilities implements provider.Provider.
func (p *Provider) Capabilities() provider.Capabilities {
	return provider.Capabilities{
		Operations: map[provider.DocumentKind][]provider.Operation{
			provider.EInvoice: {
				provider.OpSend, provider.OpStatus, provider.OpListInbox, provider.OpListOutbox,
				provider.OpFetch, provider.OpCheckUser, provider.OpMarkTaken,
			},
		},
		Formats:        []provider.Format{provider.FormatUBL, provider.FormatPDF, provider.FormatHTML},
		Protocol:       "soap",
		// The live test system rejects SendInvoice documents without a
		// ds:Signature in ExtensionContent: callers sign with their mali mühür.
		SignsDocuments: false,
		AcceptsRawUBL:  true,
		TestEndpoint:   true,
	}
}
