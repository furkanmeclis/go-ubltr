package nilvera

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/go-ubltr/provider"
)

var (
	_ provider.Sender            = (*Client)(nil)
	_ provider.StatusChecker     = (*Client)(nil)
	_ provider.InboxLister       = (*Client)(nil)
	_ provider.OutboxLister      = (*Client)(nil)
	_ provider.Fetcher           = (*Client)(nil)
	_ provider.Responder         = (*Client)(nil)
	_ provider.EArchiveCanceller = (*Client)(nil)
	_ provider.UserChecker       = (*Client)(nil)
	_ provider.Acknowledger      = (*Client)(nil)
	_ provider.CreditChecker     = (*Client)(nil)
)

func checkKind(op provider.Operation, kind provider.DocumentKind, allowed ...provider.DocumentKind) error {
	for _, k := range allowed {
		if k == kind {
			return nil
		}
	}
	return provider.Unsupported(Name, op, kind)
}

func kindOrDefault(k provider.DocumentKind) provider.DocumentKind {
	if k == "" {
		return provider.EInvoice
	}
	return k
}

func invalid(op, msg string) error {
	return &provider.Error{Kind: provider.KindInvalidRequest, Provider: Name, Op: op, Message: msg}
}

// Send implements provider.Sender with POST /einvoice/Send/Xml or
// POST /earchive/Send/Xml (multipart field "file", raw or zipped UBL-TR).
// For e-Fatura, Document.ReceiverAlias is sent as the Alias query value.
func (c *Client) Send(ctx context.Context, doc *provider.Document) (*provider.SendResult, error) {
	const op = "send"
	if doc == nil || len(doc.XML) == 0 {
		return nil, invalid(op, "empty document")
	}
	if err := doc.Inspect(); err != nil {
		return nil, &provider.Error{Kind: provider.KindValidation, Provider: Name, Op: op, Err: err}
	}
	if err := checkKind(provider.OpSend, doc.Kind, provider.EInvoice, provider.EArchive); err != nil {
		return nil, err
	}
	path := "/einvoice/Send/Xml"
	q := url.Values{}
	if doc.Kind == provider.EArchive {
		path = "/earchive/Send/Xml"
	} else if doc.ReceiverAlias != "" {
		q.Set("Alias", doc.ReceiverAlias)
	}
	name := doc.UUID
	if name == "" {
		name = "document"
	}
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", name+".xml")
	if err == nil {
		_, err = fw.Write(doc.XML)
	}
	if err == nil {
		err = mw.Close()
	}
	if err != nil {
		return nil, &provider.Error{Kind: provider.KindInvalidRequest, Provider: Name, Op: op, Err: err}
	}
	r, err := c.do(ctx, op, http.MethodPost, path, q, &buf, mw.FormDataContentType())
	if err != nil {
		return nil, err
	}
	res := &provider.SendResult{UUID: doc.UUID, ID: doc.ID, Status: provider.StatusQueued, Raw: r.body}
	var out struct {
		UUID          string `json:"UUID"`
		InvoiceNumber string `json:"InvoiceNumber"`
	}
	if len(bytes.TrimSpace(r.body)) > 0 {
		if err := json.Unmarshal(r.body, &out); err != nil {
			return nil, &provider.Error{Kind: provider.KindTransport, Provider: Name, Op: op, HTTPStatus: r.status, Raw: r.body, Message: "decode response", Err: err}
		}
		if out.UUID != "" {
			res.UUID = out.UUID
		}
		if out.InvoiceNumber != "" {
			res.ID = out.InvoiceNumber
		}
	}
	return res, nil
}

// statusResponse covers the e-Fatura (Sale / Purchase) and e-Arşiv
// StatusResponse schemas.
type statusResponse struct {
	InvoiceProfile string `json:"InvoiceProfile"`
	IssueDate      string `json:"IssueDate"`
	Answer         *struct {
		AnswerCode  string `json:"AnswerCode"`
		AnswerNote  string `json:"AnswerNote"`
		Description string `json:"Description"`
	} `json:"Answer"`
	InvoiceStatus *struct {
		Code              string `json:"Code"`
		Description       string `json:"Description"`
		DetailDescription string `json:"DetailDescription"`
	} `json:"InvoiceStatus"`
	EnvelopeInfo *struct {
		UUID           string `json:"UUID"`
		GIBCode        int    `json:"GIBCode"`
		GIBDescription string `json:"GIBDescription"`
		CreatedDate    string `json:"CreatedDate"`
	} `json:"EnvelopeInfo"`
	// e-Arşiv
	StatusDetail string `json:"StatusDetail"`
	StatusCode   string `json:"StatusCode"`
	ReportStatus string `json:"ReportStatus"`
	CancelStatus bool   `json:"CancelStatus"`
}

// Status implements provider.StatusChecker with GET /einvoice/Sale/{UUID}/Status,
// GET /einvoice/Purchase/{UUID}/Status and GET /earchive/Invoices/{UUID}/Status.
func (c *Client) Status(ctx context.Context, q provider.StatusQuery) (*provider.StatusResult, error) {
	const op = "status"
	kind := kindOrDefault(q.Kind)
	if err := checkKind(provider.OpStatus, kind, provider.EInvoice, provider.EArchive); err != nil {
		return nil, err
	}
	if q.UUID == "" {
		return nil, invalid(op, "UUID is required")
	}
	id := url.PathEscape(q.UUID)
	var path string
	switch {
	case kind == provider.EArchive:
		path = "/earchive/Invoices/" + id + "/Status"
	case q.Direction == provider.Inbound:
		path = "/einvoice/Purchase/" + id + "/Status"
	default:
		path = "/einvoice/Sale/" + id + "/Status"
	}
	var s statusResponse
	r, err := c.doJSON(ctx, op, http.MethodGet, path, nil, nil, &s)
	if err != nil {
		return nil, err
	}
	res := &provider.StatusResult{UUID: q.UUID, Raw: r.body}
	if kind == provider.EArchive {
		res.Status = ArchiveStatus(s.StatusCode, s.ReportStatus, s.CancelStatus)
		res.ProviderCode = s.StatusCode
		res.Message = s.StatusDetail
		return res, nil
	}
	code, gib := "", 0
	if s.InvoiceStatus != nil {
		code = s.InvoiceStatus.Code
		res.ProviderCode = code
		res.Message = joinMsg(s.InvoiceStatus.Description, s.InvoiceStatus.DetailDescription)
	}
	if env := s.EnvelopeInfo; env != nil {
		gib = env.GIBCode
		res.GIBCode = env.GIBCode
		if res.Message == "" {
			res.Message = env.GIBDescription
		}
		res.UpdatedAt = parseTime(env.CreatedDate)
	}
	answer := ""
	if s.Answer != nil {
		answer = s.Answer.AnswerCode
	}
	if q.Direction == provider.Inbound {
		res.Status = provider.StatusReceived
	} else {
		res.Status = InvoiceStatus(code, gib)
	}
	if st, rc, ok := answerStatus(answer); ok {
		res.Status, res.ResponseCode = st, rc
	}
	return res, nil
}

// listItem covers the Sale, Purchase and e-Arşiv listing items.
type listItem struct {
	UUID              string   `json:"UUID"`
	TaxNumber         string   `json:"TaxNumber"`
	SenderTaxNumber   string   `json:"SenderTaxNumber"`
	SenderName        string   `json:"SenderName"`
	ReceiverTaxNumber string   `json:"ReceiverTaxNumber"`
	ReceiverName      string   `json:"ReceiverName"`
	InvoiceNumber     string   `json:"InvoiceNumber"`
	InvoiceProfile    string   `json:"InvoiceProfile"`
	InvoiceType       string   `json:"InvoiceType"`
	CurrencyCode      string   `json:"CurrencyCode"`
	IssueDate         string   `json:"IssueDate"`
	PayableAmount     *float64 `json:"PayableAmount"`
	AnswerCode        string   `json:"AnswerCode"`
	StatusCode        string   `json:"StatusCode"`
	StatusDetail      string   `json:"StatusDetail"`
	CreatedDate       string   `json:"CreatedDate"`
	IsCancel          bool     `json:"IsCancel"`
	IsReport          bool     `json:"IsReport"`
}

type listPage struct {
	Page       int        `json:"Page"`
	PageSize   int        `json:"PageSize"`
	TotalCount *int       `json:"TotalCount"`
	TotalPages *int       `json:"TotalPages"`
	Content    []listItem `json:"Content"`
}

// DefaultPageSize is used when ListQuery.Limit is zero.
const DefaultPageSize = 50

func listQuery(q provider.ListQuery) url.Values {
	v := url.Values{}
	page := 1
	if n, err := strconv.Atoi(q.Cursor); err == nil && n > 0 {
		page = n
	}
	size := q.Limit
	if size <= 0 {
		size = DefaultPageSize
	}
	v.Set("Page", strconv.Itoa(page))
	v.Set("PageSize", strconv.Itoa(size))
	if !q.From.IsZero() || !q.To.IsZero() {
		v.Set("DateFilterType", "CreatedDate")
	}
	if !q.From.IsZero() {
		v.Set("StartDate", q.From.Format(time.RFC3339))
	}
	if !q.To.IsZero() {
		v.Set("EndDate", q.To.Format(time.RFC3339))
	}
	if q.OnlyNew {
		v.Set("IsTransfer", "false")
	}
	return v
}

// ListInbox implements provider.InboxLister (GET /einvoice/Purchase).
// OnlyNew filters on IsTransfer=false; MarkTaken sets Transferred.
func (c *Client) ListInbox(ctx context.Context, q provider.ListQuery) (*provider.Page, error) {
	kind := kindOrDefault(q.Kind)
	if err := checkKind(provider.OpListInbox, kind, provider.EInvoice); err != nil {
		return nil, err
	}
	return c.list(ctx, "list-inbox", kind, provider.Inbound, "/einvoice/Purchase", listQuery(q))
}

// ListOutbox implements provider.OutboxLister (GET /einvoice/Sale,
// GET /earchive/Invoices).
func (c *Client) ListOutbox(ctx context.Context, q provider.ListQuery) (*provider.Page, error) {
	kind := kindOrDefault(q.Kind)
	if err := checkKind(provider.OpListOutbox, kind, provider.EInvoice, provider.EArchive); err != nil {
		return nil, err
	}
	path := "/einvoice/Sale"
	if kind == provider.EArchive {
		path = "/earchive/Invoices"
	}
	return c.list(ctx, "list-outbox", kind, provider.Outbound, path, listQuery(q))
}

func (c *Client) list(ctx context.Context, op string, kind provider.DocumentKind, dir provider.Direction, path string, v url.Values) (*provider.Page, error) {
	var lp listPage
	if _, err := c.doJSON(ctx, op, http.MethodGet, path, v, nil, &lp); err != nil {
		return nil, err
	}
	page := &provider.Page{Items: make([]provider.DocumentInfo, 0, len(lp.Content))}
	for i := range lp.Content {
		page.Items = append(page.Items, info(kind, dir, &lp.Content[i]))
	}
	switch {
	case lp.TotalPages != nil:
		if lp.Page > 0 && lp.Page < *lp.TotalPages {
			page.Next = strconv.Itoa(lp.Page + 1)
		}
	case lp.TotalCount != nil:
		if lp.Page > 0 && lp.PageSize > 0 && lp.Page*lp.PageSize < *lp.TotalCount {
			page.Next = strconv.Itoa(lp.Page + 1)
		}
	}
	return page, nil
}

func info(kind provider.DocumentKind, dir provider.Direction, it *listItem) provider.DocumentInfo {
	d := provider.DocumentInfo{
		Kind:         kind,
		Direction:    dir,
		UUID:         it.UUID,
		ID:           it.InvoiceNumber,
		Profile:      it.InvoiceProfile,
		Type:         it.InvoiceType,
		IssueDate:    datePart(it.IssueDate),
		Currency:     it.CurrencyCode,
		CreatedAt:    parseTime(it.CreatedDate),
		ProviderCode: it.StatusCode,
	}
	if it.PayableAmount != nil {
		d.Payable = strconv.FormatFloat(*it.PayableAmount, 'f', -1, 64)
	}
	switch {
	case kind == provider.EArchive:
		d.Profile = "EARSIVFATURA"
		d.ReceiverVKN, d.ReceiverName = it.ReceiverTaxNumber, it.ReceiverName
		report := ""
		if it.IsReport {
			report = "Reported"
		}
		d.Status = ArchiveStatus(it.StatusCode, report, it.IsCancel)
	case dir == provider.Inbound:
		d.SenderVKN, d.SenderName = it.SenderTaxNumber, it.SenderName
		d.ReceiverVKN = it.TaxNumber
		d.Status = provider.StatusReceived
	default:
		d.ReceiverVKN, d.ReceiverName = it.ReceiverTaxNumber, it.ReceiverName
		d.SenderVKN = it.TaxNumber
		d.Status = InvoiceStatus(it.StatusCode, 0)
	}
	if kind != provider.EArchive {
		if st, _, ok := answerStatus(it.AnswerCode); ok {
			d.Status = st
		}
	}
	return d
}

func datePart(s string) string {
	if i := strings.IndexByte(s, 'T'); i > 0 {
		return s[:i]
	}
	return s
}

// Fetch implements provider.Fetcher with the /xml, /pdf and /html endpoints.
func (c *Client) Fetch(ctx context.Context, q provider.FetchQuery) (*provider.FetchResult, error) {
	const op = "fetch"
	kind := kindOrDefault(q.Kind)
	if err := checkKind(provider.OpFetch, kind, provider.EInvoice, provider.EArchive); err != nil {
		return nil, err
	}
	if q.UUID == "" {
		return nil, invalid(op, "UUID is required")
	}
	format := q.Format
	if format == "" {
		format = provider.FormatUBL
	}
	var suffix, ct string
	switch format {
	case provider.FormatUBL:
		suffix, ct = "xml", "application/xml"
	case provider.FormatPDF:
		suffix, ct = "pdf", "application/pdf"
	case provider.FormatHTML:
		suffix, ct = "html", "text/html"
	default:
		return nil, &provider.Error{Kind: provider.KindUnsupported, Provider: Name, Op: op, Message: "format " + string(format) + " not supported"}
	}
	var path string
	switch {
	case kind == provider.EArchive:
		path = "/earchive/Invoices/"
	case q.Direction == provider.Inbound:
		path = "/einvoice/Purchase/"
	default:
		path = "/einvoice/Sale/"
	}
	r, err := c.do(ctx, op, http.MethodGet, path+url.PathEscape(q.UUID)+"/"+suffix, nil, nil, "")
	if err != nil {
		return nil, err
	}
	if h := r.header.Get("Content-Type"); h != "" {
		ct = h
	}
	return &provider.FetchResult{Format: format, ContentType: ct, Data: r.body}, nil
}

// Respond implements provider.Responder (POST /einvoice/Purchase/SendAnswer).
func (c *Client) Respond(ctx context.Context, r provider.Response) error {
	const op = "respond"
	if r.UUID == "" {
		return invalid(op, "UUID is required")
	}
	body := struct {
		UUID       string `json:"UUID"`
		AnswerCode string `json:"AnswerCode"`
		RejectNote string `json:"RejectNote,omitempty"`
	}{UUID: r.UUID, AnswerCode: "rejected", RejectNote: r.Reason}
	if r.Accept {
		body.AnswerCode = "approved"
		body.RejectNote = ""
	}
	_, err := c.doJSON(ctx, op, http.MethodPost, "/einvoice/Purchase/SendAnswer", nil, body, nil)
	return err
}

// CancelEArchive implements provider.EArchiveCanceller
// (PUT /earchive/Invoices/{UUID}/Cancel). The API takes no reason; it is
// ignored.
func (c *Client) CancelEArchive(ctx context.Context, uuid, _ string) error {
	const op = "cancel-earchive"
	if uuid == "" {
		return invalid(op, "UUID is required")
	}
	_, err := c.doJSON(ctx, op, http.MethodPut, "/earchive/Invoices/"+url.PathEscape(uuid)+"/Cancel", nil, nil, nil)
	return err
}

// CheckUser implements provider.UserChecker
// (GET /general/GlobalCompany/Check/TaxNumber/{TaxNumber}?globalUserType=Invoice).
// An empty list or a 404 reports Registered=false.
func (c *Client) CheckUser(ctx context.Context, vknTckn string) (*provider.UserInfo, error) {
	const op = "check-user"
	vknTckn = strings.TrimSpace(vknTckn)
	if vknTckn == "" {
		return nil, invalid(op, "VKN/TCKN is required")
	}
	var rows []struct {
		TaxNumber        string `json:"TaxNumber"`
		Title            string `json:"Title"`
		FirstCreatedTime string `json:"FirstCreatedTime"`
		CreationTime     string `json:"CreationTime"`
		DocumentType     string `json:"DocumentType"`
		Name             string `json:"Name"`
		Type             string `json:"Type"`
	}
	q := url.Values{"globalUserType": {"Invoice"}}
	_, err := c.doJSON(ctx, op, http.MethodGet, "/general/GlobalCompany/Check/TaxNumber/"+url.PathEscape(vknTckn), q, nil, &rows)
	if err != nil {
		if provider.KindOf(err) == provider.KindNotFound {
			return &provider.UserInfo{VKNTCKN: vknTckn}, nil
		}
		return nil, err
	}
	u := &provider.UserInfo{VKNTCKN: vknTckn, Registered: len(rows) > 0}
	for _, r := range rows {
		if u.Title == "" {
			u.Title = r.Title
		}
		if r.Name == "" {
			continue
		}
		u.Aliases = append(u.Aliases, provider.Alias{
			Name:      r.Name,
			Type:      r.Type,
			Kind:      provider.EInvoice,
			CreatedAt: parseTime(r.CreationTime),
		})
	}
	return u, nil
}

// MarkTaken implements provider.Acknowledger for incoming e-Fatura with
// PUT /einvoice/Purchase/Operation/Transferred.
func (c *Client) MarkTaken(ctx context.Context, direction provider.Direction, uuids ...string) error {
	if direction != provider.Inbound {
		return provider.Unsupported(Name, provider.OpMarkTaken, provider.EInvoice)
	}
	if len(uuids) == 0 {
		return nil
	}
	_, err := c.doJSON(ctx, "mark-taken", http.MethodPut, "/einvoice/Purchase/Operation/Transferred", nil, uuids, nil)
	return err
}

// Credits implements provider.CreditChecker (GET /general/Credits). The
// API returns one row per credit package; Remaining and Used are summed.
func (c *Client) Credits(ctx context.Context) (*provider.CreditInfo, error) {
	var rows []struct {
		TotalCredit     int64 `json:"TotalCredit"`
		UsedCredit      int64 `json:"UsedCredit"`
		RemainingCredit int64 `json:"RemainingCredit"`
	}
	if _, err := c.doJSON(ctx, "credits", http.MethodGet, "/general/Credits", nil, nil, &rows); err != nil {
		return nil, err
	}
	ci := &provider.CreditInfo{}
	for _, r := range rows {
		ci.Remaining += r.RemainingCredit
		ci.Used += r.UsedCredit
	}
	return ci, nil
}
