package uyumsoft

import (
	"slices"
	"time"

	"github.com/furkanmeclis/go-ubltr/provider"
)

// AuthMode selects how credentials travel with each request.
type AuthMode int

// Authentication modes.
const (
	// AuthWSSecurity sends a WS-Security UsernameToken (and a Timestamp)
	// in the SOAP header, as the Uyumsoft binding policy requires
	// (sp:SignedSupportingTokens / sp:UsernameToken, sp:IncludeTimestamp).
	AuthWSSecurity AuthMode = iota
	// AuthUserInfo sends a <userInfo Username=".." Password=".."/> element
	// as the first child of every operation, as BasicIntegration does.
	AuthUserInfo
)

// Preset describes one vendor exposing the Integration contract.
type Preset struct {
	// Name is the provider name and registry key.
	Name string
	// TestURL and LiveURL are the SOAP endpoints.
	TestURL, LiveURL string
	// Auth selects the credential transport.
	Auth AuthMode
	// Contract is the SOAPAction prefix, e.g. "http://tempuri.org/IIntegration/".
	Contract string
	// Actions overrides the SOAPAction for single operations.
	Actions map[string]string
	// TakenParam is the element name of the SetInvoicesTaken id list.
	TakenParam string
	// InboxOnlyNew is true when GetInboxInvoiceList takes the
	// OnlyNewestInvoices attribute; otherwise OnlyNew filters on
	// IsArchived=false.
	InboxOnlyNew bool
	// OutboxScenario is true when GetOutboxInvoiceList takes a Scenario
	// filter; otherwise kind filtering happens on the returned page.
	OutboxScenario bool
	// IncludeTagList is true when the list query carries the mandatory
	// IncludeTagList element (Uyumsoft InvoiceListQueryModel).
	IncludeTagList bool
	// NumericStatus is true when the vendor documents its StatusCode
	// values, so they are used when the Status text is unknown.
	NumericStatus bool
	// MaxPageSize caps ListQuery.Limit (0 = no documented limit).
	MaxPageSize int
	// Capabilities lists the operations this vendor documents.
	Capabilities provider.Capabilities
}

// URL returns the endpoint for the test or live system.
func (p Preset) URL(test bool) string {
	if test {
		return p.TestURL
	}
	return p.LiveURL
}

func (p Preset) action(op string) string {
	if a, ok := p.Actions[op]; ok {
		return a
	}
	return p.Contract + op
}

// operations implemented by this module. Both vendors document every one
// of them; per-preset lists are kept separate so a vendor that drops an
// operation can be described without touching the other.
var (
	einvoiceOps = []provider.Operation{
		provider.OpSend, provider.OpStatus, provider.OpListInbox, provider.OpListOutbox,
		provider.OpFetch, provider.OpRespond, provider.OpCheckUser, provider.OpMarkTaken,
		provider.OpCredits,
	}
	earchiveOps = []provider.Operation{
		provider.OpSend, provider.OpStatus, provider.OpListOutbox, provider.OpFetch,
		provider.OpCancelEArchive, provider.OpCredits,
	}
)

// Uyumsoft is the Uyumsoft Integration service. Endpoints and binding
// come from the WSDL served at <endpoint>?singleWsdl (service
// "Integration", port BasicHttpBinding_IIntegration, SOAP 1.1).
var Uyumsoft = Preset{
	Name:     "uyumsoft",
	TestURL:  "https://efatura-test.uyumsoft.com.tr/Services/Integration",
	LiveURL:  "https://efatura.uyumsoft.com.tr/Services/Integration",
	Auth:     AuthWSSecurity,
	Contract: "http://tempuri.org/IIntegration/",
	Actions: map[string]string{
		// Inherited from ICustomerServiceBase in the WSDL binding.
		"GetCustomerCreditInfo": "http://tempuri.org/ICustomerServiceBase/GetCustomerCreditInfo",
		"GetSystemDate":         "http://tempuri.org/ICustomerServiceBase/GetSystemDate",
	},
	TakenParam:     "invoices",
	InboxOnlyNew:   true,
	OutboxScenario: true,
	IncludeTagList: true,
	Capabilities: provider.Capabilities{
		Operations: map[provider.DocumentKind][]provider.Operation{
			provider.EInvoice: slices.Clone(einvoiceOps),
			provider.EArchive: slices.Clone(earchiveOps),
		},
		Formats:       []provider.Format{provider.FormatUBL, provider.FormatPDF, provider.FormatHTML},
		Protocol:      "soap",
		AcceptsRawUBL: true,
		TestEndpoint:  true,
	},
}

// Muhasebeizleme is muhasebeizleme's BasicIntegration service (Mi API
// Kılavuzu, https://api.muhasebeizleme.com/kilavuz/). Its 32 documented
// operations are a subset of the Integration contract with the same
// names; credentials go in a userInfo element and Password is the API
// key.
var Muhasebeizleme = Preset{
	Name:          "muhasebeizleme",
	TestURL:       "https://test.api.muhasebeizleme.com/Services/BasicIntegration.svc",
	LiveURL:       "https://api.muhasebeizleme.com/Services/BasicIntegration.svc",
	Auth:          AuthUserInfo,
	Contract:      "http://tempuri.org/IBasicIntegration/",
	TakenParam:    "invoiceIds",
	NumericStatus: true,
	MaxPageSize:   1000,
	Capabilities: provider.Capabilities{
		Operations: map[provider.DocumentKind][]provider.Operation{
			provider.EInvoice: slices.Clone(einvoiceOps),
			provider.EArchive: slices.Clone(earchiveOps),
		},
		Formats:  []provider.Format{provider.FormatUBL, provider.FormatPDF, provider.FormatHTML},
		Protocol: "soap",
		// The guide: numbers and ETTN are generated by the service, the
		// UBL is sent unsigned and sealed by the service.
		AssignsNumbers:  true,
		SignsDocuments:  true,
		AcceptsRawUBL:   true,
		MinPollInterval: 5 * time.Minute,
		TestEndpoint:    true,
	},
}

// Presets lists the known presets by name.
var Presets = map[string]Preset{
	Uyumsoft.Name:       Uyumsoft,
	Muhasebeizleme.Name: Muhasebeizleme,
}
