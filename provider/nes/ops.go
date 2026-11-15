package nes

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
	"github.com/furkanmeclis/go-ubltr/provider/pack"
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

// envelopeInfo is EnvelopeInfo / OutgoingEnvelope.
type envelopeInfo struct {
	CreatedAt          string `json:"createdAt"`
	InstanceIdentifier string `json:"instanceIdentifier"`
	Code               string `json:"code"`
	Description        string `json:"description"`
	SenderIdentifier   string `json:"senderIdentifier"`
	ReceiverIdentifier string `json:"receiverIdentifier"`
}

type party struct {
	PartyIdentification string `json:"partyIdentification"`
	PartyName           string `json:"partyName"`
	FirstName           string `json:"firstName"`
	FamilyName          string `json:"familyName"`
}

func (p *party) name() string {
	if p == nil {
		return ""
	}
	if p.PartyName != "" {
		return p.PartyName
	}
	return strings.TrimSpace(p.FirstName + " " + p.FamilyName)
}

func (p *party) id() string {
	if p == nil {
		return ""
	}
	return p.PartyIdentification
}

// answer is IncomingAnswer (on outgoing documents) and OutgoingAnswer (on
// incoming documents).
type answer struct {
	DocumentAnswer         string        `json:"documentAnswer"`
	AnswerNote             string        `json:"answerNote"`
	OutgoingStatus         string        `json:"outgoingStatus"`
	ErrorDescription       string        `json:"errorDescription"`
	IncomingAnswerEnvelope *envelopeInfo `json:"incomingAnswerEnvelope"`
	OutgoingAnswerEnvelope *envelopeInfo `json:"outgoingAnswerEnvelope"`
}

// invoice holds the fields shared by the e-Fatura and e-Arşiv listing and
// detail schemas that this module reads.
type invoice struct {
	ID                    string        `json:"id"`
	CreatedAt             string        `json:"createdAt"`
	IssueDate             string        `json:"issueDate"`
	DocumentNumber        string        `json:"documentNumber"`
	InvoiceTypeCode       string        `json:"invoiceTypeCode"`
	ProfileID             string        `json:"profileId"`
	PayableAmount         *float64      `json:"payableAmount"`
	DocumentCurrencyCode  string        `json:"documentCurrencyCode"`
	DocumentAnswer        string        `json:"documentAnswer"`
	OutgoingStatus        string        `json:"outgoingStatus"`
	RecordStatus          string        `json:"recordStatus"`
	ErrorDescription      string        `json:"errorDescription"`
	SenderAlias           string        `json:"senderAlias"`
	OutgoingEnvelope      *envelopeInfo `json:"outgoingEnvelope"`
	IncomingEnvelope      *envelopeInfo `json:"incomingEnvelope"`
	IncomingAnswer        *answer       `json:"incomingAnswer"`
	OutgoingAnswer        *answer       `json:"outgoingAnswer"`
	AccountingCustomer    *party        `json:"accountingCustomerParty"`
	AccountingSupplier    *party        `json:"accountingSupplierParty"`
	IsTransferred         bool          `json:"isTransferred"`
	IsCanceled            bool          `json:"isCanceled"`
	ArchiveDocumentStatus string        `json:"archiveDocumentStatus"`
	ReportDivisionStatus  string        `json:"reportDivisionStatus"`
}

type pageResult struct {
	Page       int       `json:"page"`
	PageSize   int       `json:"pageSize"`
	TotalCount int       `json:"totalCount"`
	Data       []invoice `json:"data"`
}

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

// Send implements provider.Sender with POST /v1/uploads/document on the
// e-Fatura or e-Arşiv service. The document is sent directly
// (IsDirectSend=true), not saved as a draft. XML over MaxFileSize is zipped.
func (c *Client) Send(ctx context.Context, doc *provider.Document) (*provider.SendResult, error) {
	const op = "send"
	if doc == nil || len(doc.XML) == 0 {
		return nil, &provider.Error{Kind: provider.KindInvalidRequest, Provider: Name, Op: op, Message: "empty document"}
	}
	if err := doc.Inspect(); err != nil {
		return nil, &provider.Error{Kind: provider.KindValidation, Provider: Name, Op: op, Err: err}
	}
	if err := checkKind(provider.OpSend, doc.Kind, provider.EInvoice, provider.EArchive); err != nil {
		return nil, err
	}
	sender := doc.SenderAlias
	if sender == "" {
		sender = c.senderAlias
	}
	if doc.Kind == provider.EInvoice && sender == "" {
		return nil, &provider.Error{Kind: provider.KindInvalidRequest, Provider: Name, Op: op, Message: "SenderAlias is required for e-Fatura"}
	}
	name := doc.UUID
	if name == "" {
		name = "document"
	}
	data, fileName := doc.XML, name+".xml"
	if len(data) > MaxFileSize {
		z, err := pack.Zip(fileName, data)
		if err != nil {
			return nil, &provider.Error{Kind: provider.KindInvalidRequest, Provider: Name, Op: op, Err: err}
		}
		if len(z) > MaxFileSize {
			return nil, &provider.Error{Kind: provider.KindValidation, Provider: Name, Op: op, Message: "document exceeds the 1 MB upload limit even when zipped"}
		}
		data, fileName = z, name+".zip"
	}

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fields := [][2]string{
		{"IsDirectSend", "true"},
		{"PreviewType", "None"},
		{"SourceApp", c.sourceApp},
		{"AutoSaveCompany", "false"},
	}
	if doc.Kind == provider.EInvoice {
		fields = append(fields, [2]string{"SenderAlias", sender})
		if doc.ReceiverAlias != "" {
			fields = append(fields, [2]string{"ReceiverAlias", doc.ReceiverAlias})
		}
	}
	for _, f := range fields {
		if err := mw.WriteField(f[0], f[1]); err != nil {
			return nil, &provider.Error{Kind: provider.KindInvalidRequest, Provider: Name, Op: op, Err: err}
		}
	}
	fw, err := mw.CreateFormFile("File", fileName)
	if err == nil {
		_, err = fw.Write(data)
	}
	if err == nil {
		err = mw.Close()
	}
	if err != nil {
		return nil, &provider.Error{Kind: provider.KindInvalidRequest, Provider: Name, Op: op, Err: err}
	}

	r, err := c.do(ctx, op, http.MethodPost, service(doc.Kind)+"/v1/uploads/document", nil, &buf, mw.FormDataContentType())
	if err != nil {
		return nil, err
	}
	var out struct {
		UUID           string `json:"uuid"`
		DocumentNumber string `json:"documentNumber"`
	}
	res := &provider.SendResult{UUID: doc.UUID, ID: doc.ID, Status: provider.StatusQueued, Raw: r.body}
	if len(bytes.TrimSpace(r.body)) > 0 {
		if err := json.Unmarshal(r.body, &out); err != nil {
			return nil, &provider.Error{Kind: provider.KindTransport, Provider: Name, Op: op, HTTPStatus: r.status, Raw: r.body, Message: "decode response", Err: err}
		}
		if out.UUID != "" {
			res.UUID = out.UUID
		}
		if out.DocumentNumber != "" {
			res.ID = out.DocumentNumber
		}
	}
	return res, nil
}

// Status implements provider.StatusChecker using the document detail
// endpoints.
func (c *Client) Status(ctx context.Context, q provider.StatusQuery) (*provider.StatusResult, error) {
	const op = "status"
	kind := kindOrDefault(q.Kind)
	if err := checkKind(provider.OpStatus, kind, provider.EInvoice, provider.EArchive); err != nil {
		return nil, err
	}
	if q.UUID == "" {
		return nil, &provider.Error{Kind: provider.KindInvalidRequest, Provider: Name, Op: op, Message: "UUID is required"}
	}
	var path string
	switch {
	case kind == provider.EArchive:
		path = "/earchive/v1/invoices/" + url.PathEscape(q.UUID)
	case q.Direction == provider.Inbound:
		path = "/einvoice/v1/incoming/invoices/" + url.PathEscape(q.UUID)
	default:
		path = "/einvoice/v1/outgoing/invoices/" + url.PathEscape(q.UUID)
	}
	var inv invoice
	r, err := c.doJSON(ctx, op, http.MethodGet, path, nil, nil, &inv)
	if err != nil {
		return nil, err
	}
	res := &provider.StatusResult{UUID: inv.ID, ID: inv.DocumentNumber, Raw: r.body, UpdatedAt: parseTime(inv.CreatedAt)}
	if res.UUID == "" {
		res.UUID = q.UUID
	}
	switch {
	case kind == provider.EArchive:
		res.Status = ArchiveStatus(inv.ArchiveDocumentStatus, inv.IsCanceled, inv.ReportDivisionStatus)
		res.ProviderCode = inv.ArchiveDocumentStatus
		res.Message = inv.ErrorDescription
	case q.Direction == provider.Inbound:
		res.Status = provider.StatusReceived
		if env := inv.IncomingEnvelope; env != nil {
			res.GIBCode = gibCode(env.Code)
			res.Message = env.Description
		}
		if a := inv.OutgoingAnswer; a != nil {
			res.ProviderCode = a.DocumentAnswer
			if s, code, ok := answerStatus(a.DocumentAnswer); ok {
				res.Status, res.ResponseCode = s, code
			}
		}
	default:
		res.Status = OutgoingStatus(inv.OutgoingStatus)
		res.ProviderCode = inv.OutgoingStatus
		if env := inv.OutgoingEnvelope; env != nil {
			res.GIBCode = gibCode(env.Code)
			res.Message = env.Description
			if t := parseTime(env.CreatedAt); !t.IsZero() {
				res.UpdatedAt = t
			}
		}
		ans := inv.DocumentAnswer
		if inv.IncomingAnswer != nil && inv.IncomingAnswer.DocumentAnswer != "" {
			ans = inv.IncomingAnswer.DocumentAnswer
		}
		if s, code, ok := answerStatus(ans); ok {
			res.Status, res.ResponseCode = s, code
		}
	}
	return res, nil
}

// ListInbox implements provider.InboxLister (GET /v1/incoming/invoices).
// OnlyNew filters on erpTransferred=False; MarkTaken sets Transferred.
func (c *Client) ListInbox(ctx context.Context, q provider.ListQuery) (*provider.Page, error) {
	kind := kindOrDefault(q.Kind)
	if err := checkKind(provider.OpListInbox, kind, provider.EInvoice); err != nil {
		return nil, err
	}
	v := listQuery(q)
	if q.OnlyNew {
		v.Set("erpTransferred", "False")
	}
	return c.list(ctx, "list-inbox", kind, provider.Inbound, "/einvoice/v1/incoming/invoices", v)
}

// ListOutbox implements provider.OutboxLister (GET /v1/outgoing/invoices
// for e-Fatura, GET /v1/invoices for e-Arşiv).
func (c *Client) ListOutbox(ctx context.Context, q provider.ListQuery) (*provider.Page, error) {
	kind := kindOrDefault(q.Kind)
	if err := checkKind(provider.OpListOutbox, kind, provider.EInvoice, provider.EArchive); err != nil {
		return nil, err
	}
	path := "/einvoice/v1/outgoing/invoices"
	v := listQuery(q)
	if kind == provider.EArchive {
		path = "/earchive/v1/invoices"
		v.Set("includeCanceledDocuments", "true")
	}
	return c.list(ctx, "list-outbox", kind, provider.Outbound, path, v)
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
	v.Set("sort", "CreatedAt desc")
	v.Set("page", strconv.Itoa(page))
	v.Set("pageSize", strconv.Itoa(size))
	if !q.From.IsZero() {
		v.Set("startCreateDate", q.From.Format(time.RFC3339))
	}
	if !q.To.IsZero() {
		v.Set("endCreateDate", q.To.Format(time.RFC3339))
	}
	return v
}

func (c *Client) list(ctx context.Context, op string, kind provider.DocumentKind, dir provider.Direction, path string, v url.Values) (*provider.Page, error) {
	var pr pageResult
	if _, err := c.doJSON(ctx, op, http.MethodGet, path, v, nil, &pr); err != nil {
		return nil, err
	}
	page := &provider.Page{Items: make([]provider.DocumentInfo, 0, len(pr.Data))}
	for i := range pr.Data {
		page.Items = append(page.Items, info(kind, dir, &pr.Data[i]))
	}
	if pr.PageSize > 0 && pr.Page > 0 && pr.Page*pr.PageSize < pr.TotalCount {
		page.Next = strconv.Itoa(pr.Page + 1)
	}
	return page, nil
}

func info(kind provider.DocumentKind, dir provider.Direction, inv *invoice) provider.DocumentInfo {
	d := provider.DocumentInfo{
		Kind:      kind,
		Direction: dir,
		UUID:      inv.ID,
		ID:        inv.DocumentNumber,
		Profile:   inv.ProfileID,
		Type:      inv.InvoiceTypeCode,
		IssueDate: datePart(inv.IssueDate),
		Currency:  inv.DocumentCurrencyCode,
		CreatedAt: parseTime(inv.CreatedAt),
	}
	if inv.PayableAmount != nil {
		d.Payable = strconv.FormatFloat(*inv.PayableAmount, 'f', -1, 64)
	}
	switch {
	case kind == provider.EArchive:
		d.Profile = "EARSIVFATURA"
		d.ReceiverVKN, d.ReceiverName = inv.AccountingCustomer.id(), inv.AccountingCustomer.name()
		d.Status = ArchiveStatus(inv.ArchiveDocumentStatus, inv.IsCanceled, inv.ReportDivisionStatus)
		d.ProviderCode = inv.ArchiveDocumentStatus
	case dir == provider.Inbound:
		d.SenderVKN, d.SenderName = inv.AccountingSupplier.id(), inv.AccountingSupplier.name()
		d.Status = provider.StatusReceived
		if s, _, ok := answerStatus(inv.DocumentAnswer); ok {
			d.Status = s
		}
		d.ProviderCode = inv.DocumentAnswer
		if inv.IncomingEnvelope != nil {
			d.GIBCode = gibCode(inv.IncomingEnvelope.Code)
		}
	default:
		d.ReceiverVKN, d.ReceiverName = inv.AccountingCustomer.id(), inv.AccountingCustomer.name()
		d.Status = OutgoingStatus(inv.OutgoingStatus)
		d.ProviderCode = inv.OutgoingStatus
		if s, _, ok := answerStatus(inv.DocumentAnswer); ok {
			d.Status = s
		}
		if inv.OutgoingEnvelope != nil {
			d.GIBCode = gibCode(inv.OutgoingEnvelope.Code)
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

// Fetch implements provider.Fetcher with the xml, pdf and html endpoints.
func (c *Client) Fetch(ctx context.Context, q provider.FetchQuery) (*provider.FetchResult, error) {
	const op = "fetch"
	kind := kindOrDefault(q.Kind)
	if err := checkKind(provider.OpFetch, kind, provider.EInvoice, provider.EArchive); err != nil {
		return nil, err
	}
	if q.UUID == "" {
		return nil, &provider.Error{Kind: provider.KindInvalidRequest, Provider: Name, Op: op, Message: "UUID is required"}
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
		path = "/earchive/v1/invoices/"
	case q.Direction == provider.Inbound:
		path = "/einvoice/v1/incoming/invoices/"
	default:
		path = "/einvoice/v1/outgoing/invoices/"
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

// Respond implements provider.Responder
// (POST /v1/incoming/invoices/{uuid}/documentAnswer).
func (c *Client) Respond(ctx context.Context, r provider.Response) error {
	const op = "respond"
	if r.UUID == "" {
		return &provider.Error{Kind: provider.KindInvalidRequest, Provider: Name, Op: op, Message: "UUID is required"}
	}
	body := struct {
		AnswerNote string `json:"answerNote,omitempty"`
		Answer     string `json:"incomingInvoiceAnswerParameter"`
	}{AnswerNote: r.Reason, Answer: "RED"}
	if r.Accept {
		body.Answer = "KABUL"
	}
	_, err := c.doJSON(ctx, op, http.MethodPost, "/einvoice/v1/incoming/invoices/"+url.PathEscape(r.UUID)+"/documentAnswer", nil, body, nil)
	return err
}

// CancelEArchive implements provider.EArchiveCanceller
// (POST /v1/invoices/cancel). The API takes no reason; it is ignored.
func (c *Client) CancelEArchive(ctx context.Context, uuid, _ string) error {
	const op = "cancel-earchive"
	if uuid == "" {
		return &provider.Error{Kind: provider.KindInvalidRequest, Provider: Name, Op: op, Message: "UUID is required"}
	}
	body := struct {
		UUIDs []string `json:"uuids"`
	}{UUIDs: []string{uuid}}
	_, err := c.doJSON(ctx, op, http.MethodPost, "/earchive/v1/invoices/cancel", nil, body, nil)
	return err
}

// CheckUser implements provider.UserChecker
// (GET /v1/users/{identifier}/All). A 404 means the identifier is not in
// the GİB registered-user list and returns Registered=false.
func (c *Client) CheckUser(ctx context.Context, vknTckn string) (*provider.UserInfo, error) {
	const op = "check-user"
	vknTckn = strings.TrimSpace(vknTckn)
	if vknTckn == "" {
		return nil, &provider.Error{Kind: provider.KindInvalidRequest, Provider: Name, Op: op, Message: "VKN/TCKN is required"}
	}
	var u struct {
		Identifier string `json:"identifier"`
		Title      string `json:"title"`
		Aliases    []struct {
			Alias        string `json:"alias"`
			CreationTime string `json:"creationTime"`
			Type         string `json:"type"`
		} `json:"aliases"`
	}
	_, err := c.doJSON(ctx, op, http.MethodGet, "/einvoice/v1/users/"+url.PathEscape(vknTckn)+"/All", nil, nil, &u)
	if err != nil {
		if provider.KindOf(err) == provider.KindNotFound {
			return &provider.UserInfo{VKNTCKN: vknTckn}, nil
		}
		return nil, err
	}
	info := &provider.UserInfo{VKNTCKN: u.Identifier, Title: u.Title, Registered: true}
	if info.VKNTCKN == "" {
		info.VKNTCKN = vknTckn
	}
	for _, a := range u.Aliases {
		info.Aliases = append(info.Aliases, provider.Alias{
			Name:      a.Alias,
			Type:      strings.ToUpper(a.Type),
			Kind:      provider.EInvoice,
			CreatedAt: parseTime(a.CreationTime),
		})
	}
	return info, nil
}

// MarkTaken implements provider.Acknowledger for incoming e-Fatura with
// PUT /v1/incoming/invoices/bulk/Transferred.
func (c *Client) MarkTaken(ctx context.Context, direction provider.Direction, uuids ...string) error {
	if direction != provider.Inbound {
		return provider.Unsupported(Name, provider.OpMarkTaken, provider.EInvoice)
	}
	if len(uuids) == 0 {
		return nil
	}
	_, err := c.doJSON(ctx, "mark-taken", http.MethodPut, "/einvoice/v1/incoming/invoices/bulk/Transferred", nil, uuids, nil)
	return err
}

// Credits implements provider.CreditChecker
// (GET /general/v1/management/creditsummary). Remaining is
// totalDefinedCount - totalUsedCount - totalExpiredUnUsedCount.
func (c *Client) Credits(ctx context.Context) (*provider.CreditInfo, error) {
	var s struct {
		TotalDefinedCount       int64 `json:"totalDefinedCount"`
		TotalUsedCount          int64 `json:"totalUsedCount"`
		TotalExpiredUnUsedCount int64 `json:"totalExpiredUnUsedCount"`
	}
	if _, err := c.doJSON(ctx, "credits", http.MethodGet, "/general/v1/management/creditsummary", nil, nil, &s); err != nil {
		return nil, err
	}
	rem := s.TotalDefinedCount - s.TotalUsedCount - s.TotalExpiredUnUsedCount
	if rem < 0 {
		rem = 0
	}
	return &provider.CreditInfo{Remaining: rem, Used: s.TotalUsedCount}, nil
}
