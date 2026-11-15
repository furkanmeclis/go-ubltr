package provider

import (
	"context"
	"time"
)

// Provider is implemented by every integrator module.
type Provider interface {
	// Name is the stable module name, e.g. "uyumsoft".
	Name() string
	// Capabilities reports what this integrator supports.
	Capabilities() Capabilities
}

// Sender submits a signed-by-integrator UBL-TR document.
type Sender interface {
	Send(ctx context.Context, doc *Document) (*SendResult, error)
}

// StatusChecker returns the current status of a document.
type StatusChecker interface {
	Status(ctx context.Context, q StatusQuery) (*StatusResult, error)
}

// InboxLister lists incoming documents (gelen kutusu).
type InboxLister interface {
	ListInbox(ctx context.Context, q ListQuery) (*Page, error)
}

// OutboxLister lists outgoing documents (giden kutusu).
type OutboxLister interface {
	ListOutbox(ctx context.Context, q ListQuery) (*Page, error)
}

// Fetcher downloads a document as UBL XML, PDF or HTML.
type Fetcher interface {
	Fetch(ctx context.Context, q FetchQuery) (*FetchResult, error)
}

// Responder sends a commercial answer (kabul / red) for an incoming
// TICARIFATURA invoice.
type Responder interface {
	Respond(ctx context.Context, r Response) error
}

// EArchiveCanceller cancels an e-Arşiv invoice.
type EArchiveCanceller interface {
	CancelEArchive(ctx context.Context, uuid, reason string) error
}

// UserChecker queries the GİB registered-user list (mükellef sorgulama).
type UserChecker interface {
	CheckUser(ctx context.Context, vknTckn string) (*UserInfo, error)
}

// Acknowledger marks incoming documents as taken so the integrator stops
// returning them as new. Pollers call it after a successful hand-off.
type Acknowledger interface {
	MarkTaken(ctx context.Context, direction Direction, uuids ...string) error
}

// CreditChecker reports remaining credits (kontör).
type CreditChecker interface {
	Credits(ctx context.Context) (*CreditInfo, error)
}

// Direction is inbound or outbound.
type Direction string

// Directions.
const (
	Inbound  Direction = "inbound"
	Outbound Direction = "outbound"
)

// StatusQuery selects one document.
type StatusQuery struct {
	Kind      DocumentKind
	Direction Direction
	UUID      string
}

// StatusResult is a normalized status plus the raw vendor values.
type StatusResult struct {
	UUID         string
	ID           string
	Status       Status
	GIBCode      int
	ProviderCode string
	Message      string
	ResponseCode string // KABUL / RED for commercial invoices, when known
	UpdatedAt    time.Time
	Raw          []byte
}

// ListQuery filters a listing. Zero values mean "integrator default".
type ListQuery struct {
	Kind DocumentKind
	// From / To bound the creation or receive time.
	From, To time.Time
	// OnlyNew asks for documents not yet acknowledged with MarkTaken.
	OnlyNew bool
	// Cursor continues a previous page (Page.Next).
	Cursor string
	// Limit is a page-size hint.
	Limit int
}

// Page is one listing page.
type Page struct {
	Items []DocumentInfo
	// Next is empty when there are no more pages.
	Next string
}

// DocumentInfo is listing metadata (no document body).
type DocumentInfo struct {
	Kind         DocumentKind
	Direction    Direction
	UUID         string
	ID           string
	Profile      string
	Type         string
	IssueDate    string
	SenderVKN    string
	SenderName   string
	ReceiverVKN  string
	ReceiverName string
	Currency     string
	Payable      string
	Status       Status
	GIBCode      int
	ProviderCode string
	CreatedAt    time.Time
}

// Format is a download format.
type Format string

// Formats.
const (
	FormatUBL  Format = "ubl"
	FormatPDF  Format = "pdf"
	FormatHTML Format = "html"
)

// FetchQuery selects a document and format.
type FetchQuery struct {
	Kind      DocumentKind
	Direction Direction
	UUID      string
	Format    Format
}

// FetchResult is a downloaded document.
type FetchResult struct {
	Format      Format
	ContentType string
	Data        []byte
}

// Response is a commercial answer to an incoming invoice.
type Response struct {
	UUID   string
	Accept bool
	Reason string
}

// UserInfo is a registered e-Fatura user record.
type UserInfo struct {
	VKNTCKN    string
	Title      string
	Registered bool
	Aliases    []Alias
}

// Alias is a GİB posta kutusu / gönderici birim etiketi.
type Alias struct {
	Name      string // urn:mail:...
	Type      string // PK, GB or vendor type
	Kind      DocumentKind
	CreatedAt time.Time
}

// CreditInfo is a remaining-credit report.
type CreditInfo struct {
	Remaining int64
	Used      int64
}
