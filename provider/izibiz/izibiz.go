package izibiz

import (
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/furkanmeclis/go-ubltr/provider"
	"github.com/furkanmeclis/go-ubltr/provider/soap"
)

// Name is the registry name.
const Name = "izibiz"

// TestEndpoint is the published İzibiz test system. It serves
// AuthenticationWS, EInvoiceWS, EIArchiveWS/EFaturaArchive and
// EIrsaliyeWS/EIrsaliye below this base URL.
const TestEndpoint = "https://efaturatest.izibiz.com.tr"

// Service paths below the base URL, as published in the test WSDLs.
const (
	pathAuth     = "/AuthenticationWS"
	pathInvoice  = "/EInvoiceWS"
	pathArchive  = "/EIArchiveWS/EFaturaArchive"
	pathDespatch = "/EIrsaliyeWS/EIrsaliye"
)

// EArchiveType values for EARSIV_PROPERTIES/EARSIV_TYPE.
const (
	EArchiveNormal   = "NORMAL"
	EArchiveInternet = "INTERNET"
)

// Config configures a Provider.
type Config struct {
	// Username and Password are the İzibiz portal / web-service
	// credentials.
	Username, Password string
	// Endpoint is the base URL of the services, e.g. TestEndpoint. İzibiz
	// does not publish the production host; take it from your contract.
	Endpoint string
	// Test selects TestEndpoint when Endpoint is empty.
	Test bool
	// ApplicationName is sent as REQUEST_HEADER/APPLICATION_NAME
	// (default "go-ubltr").
	ApplicationName string
	// ChannelName is sent as REQUEST_HEADER/CHANNEL_NAME when set.
	ChannelName string
	// InvoiceSeriesPrefix, when set, is sent as SERI_PREFIX so İzibiz
	// assigns e-Fatura numbers. The prefix must be defined on the portal.
	InvoiceSeriesPrefix string
	// ArchiveSeries, when set, is sent as EARSIV_PROPERTIES/SERI so İzibiz
	// assigns e-Arşiv numbers.
	ArchiveSeries string
	// EArchiveType is EArchiveNormal (default) or EArchiveInternet.
	EArchiveType string
	// XSLTName optionally names a portal template (XSLT_NAME), e.g.
	// "DEFAULT" or "CUSTOMER_DEFAULT".
	XSLTName string
	// HTTP overrides the HTTP client.
	HTTP provider.Doer
}

// Provider is the İzibiz integrator. It is safe for concurrent use; one
// session is shared by all calls and renewed when İzibiz reports it as
// expired.
type Provider struct {
	cfg      Config
	auth     *soap.Client
	invoice  *soap.Client
	archive  *soap.Client
	despatch *soap.Client
	codes    *provider.CodeTable

	mu      sync.Mutex
	session string
	gen     uint64
	now     func() time.Time
}

var (
	_ provider.Provider          = (*Provider)(nil)
	_ provider.Sender            = (*Provider)(nil)
	_ provider.StatusChecker     = (*Provider)(nil)
	_ provider.InboxLister       = (*Provider)(nil)
	_ provider.OutboxLister      = (*Provider)(nil)
	_ provider.Fetcher           = (*Provider)(nil)
	_ provider.Responder         = (*Provider)(nil)
	_ provider.EArchiveCanceller = (*Provider)(nil)
	_ provider.UserChecker       = (*Provider)(nil)
	_ provider.Acknowledger      = (*Provider)(nil)
)

// New returns a Provider.
func New(cfg Config) (*Provider, error) {
	if cfg.Username == "" || cfg.Password == "" {
		return nil, errors.New("izibiz: username and password are required")
	}
	base := strings.TrimRight(cfg.Endpoint, "/")
	if base == "" {
		if !cfg.Test {
			return nil, errors.New("izibiz: Endpoint is required outside the test system")
		}
		base = TestEndpoint
	}
	if cfg.ApplicationName == "" {
		cfg.ApplicationName = "go-ubltr"
	}
	switch cfg.EArchiveType {
	case "":
		cfg.EArchiveType = EArchiveNormal
	case EArchiveNormal, EArchiveInternet:
	default:
		return nil, errors.New("izibiz: EArchiveType must be NORMAL or INTERNET")
	}
	doer := cfg.HTTP
	if doer == nil {
		doer = &http.Client{Timeout: 2 * time.Minute}
	}
	client := func(path string) *soap.Client {
		return &soap.Client{URL: base + path, HTTP: doer, Provider: Name}
	}
	return &Provider{
		cfg:      cfg,
		auth:     client(pathAuth),
		invoice:  client(pathInvoice),
		archive:  client(pathArchive),
		despatch: client(pathDespatch),
		codes:    Codes,
		now:      time.Now,
	}, nil
}

// Open is the registry factory. Options: "application_name",
// "channel_name", "invoice_series_prefix", "archive_series",
// "earchive_type", "xslt_name".
func Open(c provider.Config) (provider.Provider, error) {
	o := c.Options
	return New(Config{
		Username:            c.Username,
		Password:            c.Password,
		Endpoint:            c.Endpoint,
		Test:                c.Test,
		ApplicationName:     o["application_name"],
		ChannelName:         o["channel_name"],
		InvoiceSeriesPrefix: o["invoice_series_prefix"],
		ArchiveSeries:       o["archive_series"],
		EArchiveType:        strings.ToUpper(o["earchive_type"]),
		XSLTName:            o["xslt_name"],
		HTTP:                c.HTTP,
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
				provider.OpFetch, provider.OpRespond, provider.OpCheckUser, provider.OpMarkTaken,
			},
			provider.EArchive: {
				provider.OpSend, provider.OpStatus, provider.OpListOutbox, provider.OpFetch, provider.OpCancelEArchive,
			},
			provider.EDespatch: {
				provider.OpSend, provider.OpStatus,
			},
		},
		Formats:        []provider.Format{provider.FormatUBL, provider.FormatPDF, provider.FormatHTML},
		Protocol:       "soap",
		AssignsNumbers: p.cfg.InvoiceSeriesPrefix != "" || p.cfg.ArchiveSeries != "",
		SignsDocuments: true,
		AcceptsRawUBL:  true,
		// "Bir istek boyutu maksimum 5MB veya 100 adet belge içerebilir."
		MaxBatch: 100,
		// GetInvoice polling "en az 15 dk"; status polling "en az 4 saatte bir".
		MinPollInterval: 15 * time.Minute,
		TestEndpoint:    true,
	}
}
