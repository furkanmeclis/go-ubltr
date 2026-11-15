package uyumsoft

import (
	"context"
	"encoding/xml"
	"errors"
	"strconv"
	"strings"

	"github.com/furkanmeclis/go-ubltr/provider"
	"github.com/furkanmeclis/go-ubltr/provider/pack"
)

var (
	errEmptyDocument = errors.New("uyumsoft: empty document")
	errNotInvoice    = errors.New("uyumsoft: only Invoice documents (e-Fatura / e-Arşiv) can be sent")
)

func (c *Client) invalid(op, msg string) error {
	return &provider.Error{Kind: provider.KindInvalidRequest, Provider: c.preset.Name, Op: op, Message: msg}
}

// stringArray is tns:ArrayOfString.
type stringArray struct {
	Strings []string `xml:"string"`
}

// idsRequest is an operation taking userInfo + one ArrayOfString.
type idsRequest struct {
	XMLName  xml.Name
	UserInfo *userInfo   `xml:"userInfo,omitempty"`
	IDs      stringArray `xml:"invoiceIds"`
}

// idRequest is an operation taking userInfo + invoiceId.
type idRequest struct {
	XMLName   xml.Name
	UserInfo  *userInfo `xml:"userInfo,omitempty"`
	InvoiceID string    `xml:"invoiceId"`
}

func opName(op string) xml.Name { return xml.Name{Space: nsTempuri, Local: op} }

// ---- Status ----

// statusResult is InvoiceStatusResponse / DisApiCevapOfListOfInvoiceStatusInfo.
type statusResult struct {
	result
	Value []struct {
		InvoiceID          string `xml:"InvoiceId,attr"`
		Status             string `xml:"Status,attr"`
		StatusCode         int    `xml:"StatusCode,attr"`
		Message            string `xml:"Message,attr"`
		EnvelopeStatusCode int    `xml:"EnvelopeStatusCode,attr"`
	} `xml:"Value"`
}

// Status implements provider.StatusChecker with QueryOutboxInvoiceStatus
// or QueryInboxInvoiceStatus.
func (c *Client) Status(ctx context.Context, q provider.StatusQuery) (*provider.StatusResult, error) {
	op := "QueryOutboxInvoiceStatus"
	if q.Direction == provider.Inbound {
		op = "QueryInboxInvoiceStatus"
		if q.Kind == provider.EArchive {
			return nil, provider.Unsupported(c.preset.Name, provider.OpStatus, q.Kind)
		}
	}
	if err := c.supports(q.Kind, provider.OpStatus); err != nil {
		return nil, err
	}
	if q.UUID == "" {
		return nil, c.invalid(op, "UUID is required")
	}
	req := idsRequest{XMLName: opName(op), UserInfo: c.userInfo(), IDs: stringArray{Strings: []string{q.UUID}}}
	res, raw, err := call[statusResult](ctx, c, op, &req)
	if err != nil {
		return nil, err
	}
	for _, v := range res.Value {
		if v.InvoiceID != "" && !strings.EqualFold(v.InvoiceID, q.UUID) {
			continue
		}
		if v.Status == "Unknown" || (v.Status == "" && v.Message != "" && classifyMessage(v.Message) == classNotFound) {
			e := Codes.NewError(c.preset.Name, op, "status:Unknown", v.Message)
			e.Code, e.Raw = v.Status, raw
			return nil, e
		}
		dir := q.Direction
		if dir == "" {
			dir = provider.Outbound
		}
		in := statusInput{Raw: v.Status, Code: v.StatusCode, Direction: dir}
		return &provider.StatusResult{
			UUID:         q.UUID,
			Status:       mapStatus(c.preset, in),
			GIBCode:      gibCode("", v.EnvelopeStatusCode),
			ProviderCode: v.Status,
			ResponseCode: responseCode(in),
			Message:      v.Message,
			Raw:          raw,
		}, nil
	}
	return nil, &provider.Error{Kind: provider.KindNotFound, Provider: c.preset.Name, Op: op, Message: "no status for " + q.UUID, Raw: raw}
}

// ---- Listing ----

// listQuery is InvoiceListQueryModel and its Inbox/Outbox subtypes. The
// element order is the WSDL sequence; empty values are omitted (both
// services treat a missing nillable element as null).
type listQuery struct {
	PageIndex          int      `xml:"PageIndex,attr"`
	PageSize           int      `xml:"PageSize,attr"`
	OnlyNewestInvoices *bool    `xml:"OnlyNewestInvoices,attr,omitempty"`
	ExecutionStartDate string   `xml:"ExecutionStartDate,omitempty"`
	ExecutionEndDate   string   `xml:"ExecutionEndDate,omitempty"`
	CreateStartDate    string   `xml:"CreateStartDate,omitempty"`
	CreateEndDate      string   `xml:"CreateEndDate,omitempty"`
	InvoiceIDs         []string `xml:"InvoiceIds,omitempty"`
	IsArchived         *bool    `xml:"IsArchived,omitempty"`
	IncludeTagList     *bool    `xml:"IncludeTagList,omitempty"`
	Scenario           string   `xml:"Scenario,omitempty"`
}

type listRequest struct {
	XMLName  xml.Name
	UserInfo *userInfo `xml:"userInfo,omitempty"`
	Query    listQuery `xml:"query"`
}

// listItem covers InboxInvoiceListItem / OutboxInvoiceListItem (Uyumsoft)
// and InvoiceListItem (BasicIntegration).
type listItem struct {
	InvoiceID            string `xml:"InvoiceId"`
	DocumentID           string `xml:"DocumentId"`
	Type                 string `xml:"Type"`
	TargetTcknVkn        string `xml:"TargetTcknVkn"`
	TargetTitle          string `xml:"TargetTitle"`
	EnvelopeIdentifier   string `xml:"EnvelopeIdentifier"`
	Status               string `xml:"Status"`
	StatusCode           int    `xml:"StatusCode"`
	EnvelopeStatus       string `xml:"EnvelopeStatus"`
	EnvelopeStatusCode   string `xml:"EnvelopeStatusCode"`
	Message              string `xml:"Message"`
	CreateDateUtc        string `xml:"CreateDateUtc"`
	ExecutionDate        string `xml:"ExecutionDate"`
	PayableAmount        string `xml:"PayableAmount"`
	DocumentCurrencyCode string `xml:"DocumentCurrencyCode"`
	Scenario             string `xml:"Scenario"`
	Profile              string `xml:"Profile"`
	InvoiceTipType       string `xml:"InvoiceTipType"`
	LocalDocumentID      string `xml:"LocalDocumentId"`
}

type listResult struct {
	result
	Value *struct {
		PageIndex  int        `xml:"PageIndex,attr"`
		PageSize   int        `xml:"PageSize,attr"`
		TotalCount int        `xml:"TotalCount,attr"`
		TotalPages int        `xml:"TotalPages,attr"`
		Items      []listItem `xml:"Items"`
	} `xml:"Value"`
}

// DefaultPageSize is used when ListQuery.Limit is zero.
const DefaultPageSize = 100

// ListInbox implements provider.InboxLister with GetInboxInvoiceList.
// OnlyNew sends OnlyNewestInvoices=true (Uyumsoft) or IsArchived=false
// (BasicIntegration): documents not yet marked with MarkTaken. Because
// MarkTaken removes documents from that set, restart from the first page
// after acknowledging instead of following Next.
func (c *Client) ListInbox(ctx context.Context, q provider.ListQuery) (*provider.Page, error) {
	if q.Kind == provider.EArchive {
		return nil, provider.Unsupported(c.preset.Name, provider.OpListInbox, q.Kind)
	}
	return c.list(ctx, "GetInboxInvoiceList", provider.Inbound, q)
}

// ListOutbox implements provider.OutboxLister with GetOutboxInvoiceList.
// The outbox holds both e-Fatura and e-Arşiv; Kind filters on the Scenario
// (server side where the WSDL allows it, otherwise on the returned page).
func (c *Client) ListOutbox(ctx context.Context, q provider.ListQuery) (*provider.Page, error) {
	return c.list(ctx, "GetOutboxInvoiceList", provider.Outbound, q)
}

func (c *Client) list(ctx context.Context, op string, dir provider.Direction, q provider.ListQuery) (*provider.Page, error) {
	opc := provider.OpListOutbox
	if dir == provider.Inbound {
		opc = provider.OpListInbox
	}
	if err := c.supports(q.Kind, opc); err != nil {
		return nil, err
	}
	page := 0
	if q.Cursor != "" {
		n, err := strconv.Atoi(q.Cursor)
		if err != nil || n < 0 {
			return nil, c.invalid(op, "invalid cursor "+strconv.Quote(q.Cursor))
		}
		page = n
	}
	size := q.Limit
	if size <= 0 {
		size = DefaultPageSize
	}
	if c.preset.MaxPageSize > 0 && size > c.preset.MaxPageSize {
		size = c.preset.MaxPageSize
	}
	lq := listQuery{PageIndex: page, PageSize: size}
	if !q.From.IsZero() {
		lq.CreateStartDate = formatLocal(q.From)
	}
	if !q.To.IsZero() {
		lq.CreateEndDate = formatLocal(q.To)
	}
	if c.preset.IncludeTagList {
		f := false
		lq.IncludeTagList = &f
	}
	if q.OnlyNew {
		if dir != provider.Inbound {
			return nil, c.invalid(op, "OnlyNew applies to the inbox only")
		}
		t, f := true, false
		if c.preset.InboxOnlyNew {
			lq.OnlyNewestInvoices = &t
		} else {
			lq.IsArchived = &f
		}
	} else if dir == provider.Inbound && c.preset.InboxOnlyNew {
		f := false
		lq.OnlyNewestInvoices = &f
	}
	if dir == provider.Outbound && c.preset.OutboxScenario {
		switch q.Kind {
		case provider.EInvoice:
			lq.Scenario = "eInvoice"
		case provider.EArchive:
			lq.Scenario = "eArchive"
		}
	}
	req := listRequest{XMLName: opName(op), UserInfo: c.userInfo(), Query: lq}
	res, _, err := call[listResult](ctx, c, op, &req)
	if err != nil {
		return nil, err
	}
	out := &provider.Page{}
	if res.Value == nil {
		return out, nil
	}
	for _, it := range res.Value.Items {
		info := c.documentInfo(dir, it)
		if q.Kind != "" && info.Kind != q.Kind {
			continue
		}
		out.Items = append(out.Items, info)
	}
	v := res.Value
	switch {
	case v.TotalPages > 0:
		if v.PageIndex+1 < v.TotalPages {
			out.Next = strconv.Itoa(v.PageIndex + 1)
		}
	case len(v.Items) >= size:
		out.Next = strconv.Itoa(page + 1)
	}
	return out, nil
}

func (c *Client) documentInfo(dir provider.Direction, it listItem) provider.DocumentInfo {
	profile := it.Profile
	if profile == "" {
		profile = profiles[it.Type]
	}
	kind := provider.EInvoice
	if dir == provider.Outbound && (it.Scenario == "eArchive" || it.Type == "eArchive" || profile == "EARSIVFATURA") {
		kind = provider.EArchive
	}
	in := statusInput{Raw: it.Status, Code: it.StatusCode, Direction: dir, Commercial: profile == "TICARIFATURA"}
	envCode, _ := strconv.Atoi(strings.TrimSpace(it.EnvelopeStatusCode))
	d := provider.DocumentInfo{
		Kind:         kind,
		Direction:    dir,
		UUID:         it.DocumentID, // list rows: DocumentId is the ETTN
		ID:           it.InvoiceID,  // list rows: InvoiceId is the invoice number
		Profile:      profile,
		Type:         invoiceTypes[it.InvoiceTipType],
		Currency:     it.DocumentCurrencyCode,
		Payable:      it.PayableAmount,
		Status:       mapStatus(c.preset, in),
		GIBCode:      gibCode(it.EnvelopeStatus, envCode),
		ProviderCode: it.Status,
		CreatedAt:    parseTime(it.CreateDateUtc),
	}
	if t := parseTime(it.ExecutionDate); !t.IsZero() {
		d.IssueDate = t.Format("2006-01-02")
	}
	if dir == provider.Inbound {
		d.SenderVKN, d.SenderName = it.TargetTcknVkn, it.TargetTitle
	} else {
		d.ReceiverVKN, d.ReceiverName = it.TargetTcknVkn, it.TargetTitle
	}
	return d
}

// ---- Fetch ----

type dataResult struct {
	result
	Value *struct {
		Data string `xml:"Data"`
		HTML string `xml:"Html"`
	} `xml:"Value"`
}

// Fetch implements provider.Fetcher with Get{Inbox,Outbox}Invoice{Data,
// Pdf,View}. Data is base64 (zipped content is unpacked); the Uyumsoft
// view returns HTML text in Value/Html, BasicIntegration base64 in Data.
func (c *Client) Fetch(ctx context.Context, q provider.FetchQuery) (*provider.FetchResult, error) {
	box := "Outbox"
	if q.Direction == provider.Inbound {
		if q.Kind == provider.EArchive {
			return nil, provider.Unsupported(c.preset.Name, provider.OpFetch, q.Kind)
		}
		box = "Inbox"
	}
	if err := c.supports(q.Kind, provider.OpFetch); err != nil {
		return nil, err
	}
	format := q.Format
	if format == "" {
		format = provider.FormatUBL
	}
	var suffix, ctype string
	switch format {
	case provider.FormatUBL:
		suffix, ctype = "Data", "application/xml"
	case provider.FormatPDF:
		suffix, ctype = "Pdf", "application/pdf"
	case provider.FormatHTML:
		suffix, ctype = "View", "text/html; charset=utf-8"
	default:
		return nil, c.invalid("Fetch", "unsupported format "+string(format))
	}
	op := "Get" + box + "Invoice" + suffix
	if q.UUID == "" {
		return nil, c.invalid(op, "UUID is required")
	}
	req := idRequest{XMLName: opName(op), UserInfo: c.userInfo(), InvoiceID: q.UUID}
	res, raw, err := call[dataResult](ctx, c, op, &req)
	if err != nil {
		return nil, err
	}
	if res.Value == nil || (res.Value.Data == "" && res.Value.HTML == "") {
		return nil, &provider.Error{Kind: provider.KindNotFound, Provider: c.preset.Name, Op: op, Message: "empty document", Raw: raw}
	}
	var data []byte
	if format == provider.FormatHTML && res.Value.HTML != "" {
		data = []byte(res.Value.HTML)
	} else {
		data, err = pack.FromBase64(res.Value.Data)
		if err != nil {
			return nil, &provider.Error{Kind: provider.KindTransport, Provider: c.preset.Name, Op: op, Message: "invalid base64", Raw: raw, Err: err}
		}
	}
	if pack.IsZip(data) {
		if _, b, zerr := pack.UnzipOne(data); zerr == nil {
			data = b
		}
	}
	return &provider.FetchResult{Format: format, ContentType: ctype, Data: data}, nil
}

// ---- Respond ----

type documentResponseRequest struct {
	XMLName   xml.Name  `xml:"http://tempuri.org/ SendDocumentResponse"`
	UserInfo  *userInfo `xml:"userInfo,omitempty"`
	Responses struct {
		Items []documentResponseInfo `xml:"DocumentResponseInfo"`
	} `xml:"responses"`
}

type documentResponseInfo struct {
	InvoiceID      string `xml:"InvoiceId"`
	ResponseStatus string `xml:"ResponseStatus"`
	Reason         string `xml:"Reason,omitempty"`
}

// respondResult is FlagResponse (Uyumsoft, Value attribute) or
// DisApiCevapOfListOfDocumentResponseResult (BasicIntegration, Value
// elements with per-document IsSucceded).
type respondResult struct {
	flagResult
	Items []struct {
		InvoiceID  string `xml:"InvoiceId"`
		IsSucceded bool   `xml:"IsSucceded"`
		Message    string `xml:"Message"`
	} `xml:"Value"`
}

// Respond implements provider.Responder with SendDocumentResponse
// (ResponseStatus Approved = KABUL, Declined = RED).
func (c *Client) Respond(ctx context.Context, r provider.Response) error {
	const op = "SendDocumentResponse"
	if r.UUID == "" {
		return c.invalid(op, "UUID is required")
	}
	st := "Declined"
	if r.Accept {
		st = "Approved"
	}
	req := documentResponseRequest{UserInfo: c.userInfo()}
	req.Responses.Items = []documentResponseInfo{{InvoiceID: r.UUID, ResponseStatus: st, Reason: r.Reason}}
	res, raw, err := call[respondResult](ctx, c, op, &req)
	if err != nil {
		return err
	}
	if err := c.checkFlag(op, &res.flagResult, raw); err != nil {
		return err
	}
	for _, it := range res.Items {
		if !it.IsSucceded {
			e := messageError(c.preset.Name, op, it.Message)
			e.Raw = raw
			return e
		}
	}
	return nil
}

// ---- e-Arşiv cancel ----

type cancelRequest struct {
	XMLName  xml.Name  `xml:"http://tempuri.org/ CancelEArchiveInvoice"`
	UserInfo *userInfo `xml:"userInfo,omitempty"`
	Request  struct {
		InvoiceID  string `xml:"InvoiceId,attr"`
		CancelDate string `xml:"CancelDate,attr"`
	} `xml:"request"`
}

// CancelEArchive implements provider.EArchiveCanceller with
// CancelEArchiveInvoice. EArchiveCancelInvoiceContext has no reason field,
// so reason is not sent. Value="false" (e.g. cancel window closed) is an
// error classified from Message.
func (c *Client) CancelEArchive(ctx context.Context, uuid, _ string) error {
	const op = "CancelEArchiveInvoice"
	if uuid == "" {
		return c.invalid(op, "UUID is required")
	}
	req := cancelRequest{UserInfo: c.userInfo()}
	req.Request.InvoiceID = uuid
	req.Request.CancelDate = formatLocal(c.now())
	res, raw, err := call[flagResult](ctx, c, op, &req)
	if err != nil {
		return err
	}
	return c.checkFlag(op, res, raw)
}

// ---- Users ----

type userRequest struct {
	XMLName  xml.Name
	UserInfo *userInfo `xml:"userInfo,omitempty"`
	VknTckn  string    `xml:"vknTckn"`
}

type aliasEntry struct {
	Alias            string `xml:"Alias,attr"`
	Type             int    `xml:"Type,attr"`
	SystemCreateDate string `xml:"SystemCreateDate,attr"`
	Enabled          *bool  `xml:"Enabled,attr"`
}

type aliasResult struct {
	result
	Value *struct {
		Definition struct {
			Identifier string `xml:"Identifier,attr"`
			Title      string `xml:"Title,attr"`
		} `xml:"Definition"`
		Receiverbox         []aliasEntry `xml:"ReceiverboxAliases"`
		Senderbox           []aliasEntry `xml:"SenderboxAliases"`
		DespatchReceiverbox []aliasEntry `xml:"DespatchReceiverboxAliases"`
		DespatchSenderbox   []aliasEntry `xml:"DespatchSenderboxAliases"`
	} `xml:"Value"`
}

// CheckUser implements provider.UserChecker with IsEInvoiceUser and, for
// registered users, GetUserAliasses. Receiver boxes are type "PK", sender
// units "GB"; e-İrsaliye boxes are reported with Kind EDespatch. Disabled
// aliases are skipped.
func (c *Client) CheckUser(ctx context.Context, vknTckn string) (*provider.UserInfo, error) {
	if vknTckn == "" {
		return nil, c.invalid("IsEInvoiceUser", "VKN/TCKN is required")
	}
	req := userRequest{XMLName: opName("IsEInvoiceUser"), UserInfo: c.userInfo(), VknTckn: vknTckn}
	flag, _, err := call[flagResult](ctx, c, "IsEInvoiceUser", &req)
	if err != nil {
		return nil, err
	}
	u := &provider.UserInfo{VKNTCKN: vknTckn, Registered: flag.Value != nil && *flag.Value}
	if !u.Registered {
		return u, nil
	}
	req.XMLName = opName("GetUserAliasses")
	al, _, err := call[aliasResult](ctx, c, "GetUserAliasses", &req)
	if err != nil {
		return nil, err
	}
	if al.Value == nil {
		return u, nil
	}
	u.Title = al.Value.Definition.Title
	add := func(list []aliasEntry, typ string, kind provider.DocumentKind) {
		for _, a := range list {
			if a.Alias == "" || (a.Enabled != nil && !*a.Enabled) {
				continue
			}
			u.Aliases = append(u.Aliases, provider.Alias{Name: a.Alias, Type: typ, Kind: kind, CreatedAt: parseTime(a.SystemCreateDate)})
		}
	}
	add(al.Value.Receiverbox, "PK", provider.EInvoice)
	add(al.Value.Senderbox, "GB", provider.EInvoice)
	add(al.Value.DespatchReceiverbox, "PK", provider.EDespatch)
	add(al.Value.DespatchSenderbox, "GB", provider.EDespatch)
	return u, nil
}

// ---- Mark taken ----

type takenRequest struct {
	XMLName    xml.Name     `xml:"http://tempuri.org/ SetInvoicesTaken"`
	UserInfo   *userInfo    `xml:"userInfo,omitempty"`
	Invoices   *stringArray `xml:"invoices,omitempty"`
	InvoiceIDs *stringArray `xml:"invoiceIds,omitempty"`
}

// MarkTaken implements provider.Acknowledger with SetInvoicesTaken
// (incoming invoices only).
func (c *Client) MarkTaken(ctx context.Context, direction provider.Direction, uuids ...string) error {
	const op = "SetInvoicesTaken"
	if direction != provider.Inbound {
		return provider.Unsupported(c.preset.Name, provider.OpMarkTaken, provider.EInvoice)
	}
	if len(uuids) == 0 {
		return nil
	}
	ids := &stringArray{Strings: uuids}
	req := takenRequest{UserInfo: c.userInfo()}
	if c.preset.TakenParam == "invoiceIds" {
		req.InvoiceIDs = ids
	} else {
		req.Invoices = ids
	}
	res, raw, err := call[flagResult](ctx, c, op, &req)
	if err != nil {
		return err
	}
	return c.checkFlag(op, res, raw)
}

// ---- Credits ----

// Credit is one CustomerCreditInfo row.
type Credit struct {
	VknTckn     string
	Product     string
	Credits     int64
	IsPrepaid   bool
	IsNeedToBuy bool
	// Unit is QuantityUnitEnum (Adet, Sayfa, GB).
	Unit string
}

type creditResult struct {
	result
	Value []struct {
		VknTckn          string `xml:"VknTckn"`
		Product          string `xml:"Product"`
		Credits          int64  `xml:"Credits"`
		IsPrepaid        bool   `xml:"IsPrepaid"`
		IsNeedToBuy      bool   `xml:"IsNeedToBuy"`
		QuantityUnitEnum string `xml:"QuantityUnitEnum"`
	} `xml:"Value"`
}

type emptyRequest struct {
	XMLName  xml.Name
	UserInfo *userInfo `xml:"userInfo,omitempty"`
}

// CreditDetails returns every GetCustomerCreditInfo row.
func (c *Client) CreditDetails(ctx context.Context) ([]Credit, error) {
	const op = "GetCustomerCreditInfo"
	res, _, err := call[creditResult](ctx, c, op, &emptyRequest{XMLName: opName(op), UserInfo: c.userInfo()})
	if err != nil {
		return nil, err
	}
	out := make([]Credit, 0, len(res.Value))
	for _, v := range res.Value {
		out = append(out, Credit{VknTckn: v.VknTckn, Product: v.Product, Credits: v.Credits, IsPrepaid: v.IsPrepaid, IsNeedToBuy: v.IsNeedToBuy, Unit: v.QuantityUnitEnum})
	}
	return out, nil
}

// Credits implements provider.CreditChecker: the sum of rows counted in
// documents (QuantityUnitEnum Adet or empty). Used is not reported.
func (c *Client) Credits(ctx context.Context) (*provider.CreditInfo, error) {
	rows, err := c.CreditDetails(ctx)
	if err != nil {
		return nil, err
	}
	ci := &provider.CreditInfo{}
	for _, r := range rows {
		if r.Unit == "" || r.Unit == "Adet" {
			ci.Remaining += r.Credits
		}
	}
	return ci, nil
}

// ---- Misc ----

// Ping calls TestConnection and reports whether the credentials work.
func (c *Client) Ping(ctx context.Context) error {
	const op = "TestConnection"
	_, _, err := call[result](ctx, c, op, &emptyRequest{XMLName: opName(op), UserInfo: c.userInfo()})
	return err
}
