package turkcell

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/furkanmeclis/go-ubltr/provider"
)

var (
	_ provider.Sender            = (*Client)(nil)
	_ provider.StatusChecker     = (*Client)(nil)
	_ provider.InboxLister       = (*Client)(nil)
	_ provider.Fetcher           = (*Client)(nil)
	_ provider.Responder         = (*Client)(nil)
	_ provider.EArchiveCanceller = (*Client)(nil)
	_ provider.Acknowledger      = (*Client)(nil)
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

// Send implements provider.Sender with POST /v2/outboxinvoice (e-Fatura,
// appType=1) or POST /v2/earchive, multipart field invoiceFile, status=20
// (Kaydet ve Gönder). When the XML carries cbc:ID, useManualInvoiceId=true
// keeps it; otherwise the system assigns the number. For e-Arşiv,
// Document.Email sets sendEMail / eMailAddress.
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
	fields := [][2]string{
		{"status", "20"},
		{"useManualInvoiceId", strconv.FormatBool(doc.ID != "")},
	}
	path := "/v2/outboxinvoice"
	if doc.Kind == provider.EArchive {
		path = "/v2/earchive"
		if doc.Email != "" {
			fields = append(fields, [2]string{"sendEMail", "true"}, [2]string{"eMailAddress", doc.Email})
		}
	} else {
		fields = append(fields, [2]string{"appType", "1"})
		if doc.ReceiverAlias != "" {
			fields = append(fields, [2]string{"targetAlias", doc.ReceiverAlias})
		}
	}
	name := doc.UUID
	if name == "" {
		name = "invoice"
	}
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	var err error
	for _, f := range fields {
		if err = mw.WriteField(f[0], f[1]); err != nil {
			break
		}
	}
	if err == nil {
		fw, ferr := mw.CreateFormFile("invoiceFile", name+".xml")
		if err = ferr; err == nil {
			_, err = fw.Write(doc.XML)
		}
	}
	if err == nil {
		err = mw.Close()
	}
	if err != nil {
		return nil, &provider.Error{Kind: provider.KindInvalidRequest, Provider: Name, Op: op, Err: err}
	}
	r, err := c.do(ctx, op, http.MethodPost, path, nil, &buf, mw.FormDataContentType())
	if err != nil {
		return nil, err
	}
	res := &provider.SendResult{UUID: doc.UUID, ID: doc.ID, Status: provider.StatusQueued, Raw: r.body}
	var out struct {
		ID            string `json:"Id"`
		InvoiceNumber string `json:"InvoiceNumber"`
	}
	if len(bytes.TrimSpace(r.body)) > 0 {
		if err := json.Unmarshal(r.body, &out); err != nil {
			return nil, &provider.Error{Kind: provider.KindTransport, Provider: Name, Op: op, HTTPStatus: r.status, Raw: r.body, Message: "decode response", Err: err}
		}
		if out.ID != "" {
			res.UUID = out.ID
		}
		if out.InvoiceNumber != "" {
			res.ID = out.InvoiceNumber
		}
	}
	return res, nil
}

// Status implements provider.StatusChecker with GET /v2/outboxinvoice/{id}/status,
// GET /v2/inboxinvoice/{id}/status and GET /v2/earchive/{id}/status. The
// {id} is the ETTN.
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
	inbound := kind == provider.EInvoice && q.Direction == provider.Inbound
	var path string
	switch {
	case kind == provider.EArchive:
		path = "/v2/earchive/" + id + "/status"
	case inbound:
		path = "/v2/inboxinvoice/" + id + "/status"
	default:
		path = "/v2/outboxinvoice/" + id + "/status"
	}
	// Outbox / e-Arşiv use camelCase, inbox uses PascalCase; encoding/json
	// matches keys case-insensitively.
	var s struct {
		ID              string  `json:"id"`
		InvoiceNumber   string  `json:"invoiceNumber"`
		Status          flexInt `json:"status"`
		Message         string  `json:"message"`
		EnvelopeID      string  `json:"envelopeId"`
		EnvelopeStatus  flexInt `json:"envelopeStatus"`
		EnvelopeMessage string  `json:"envelopeMessage"`
	}
	r, err := c.doJSON(ctx, op, http.MethodGet, path, nil, nil, &s)
	if err != nil {
		return nil, err
	}
	res := &provider.StatusResult{UUID: s.ID, ID: s.InvoiceNumber, ProviderCode: s.Status.raw, Message: s.Message, Raw: r.body}
	if res.UUID == "" {
		res.UUID = q.UUID
	}
	switch {
	case !s.Status.set:
		res.Status = provider.StatusUnknown
		if inbound {
			res.Status = provider.StatusReceived
		}
	case inbound:
		res.Status = InboxStatus(s.Status.n)
	default:
		res.Status = OutboxStatus(s.Status.n)
	}
	res.ResponseCode = responseCode(res.Status)
	if s.EnvelopeStatus.set {
		res.GIBCode = envelopeGIB(s.EnvelopeStatus.n)
		if g := provider.GIBStatus(res.GIBCode); g == provider.StatusFailed && res.Status != provider.StatusFailed {
			res.Status = provider.StatusFailed
		}
	}
	if res.Message == "" {
		res.Message = s.EnvelopeMessage
	}
	return res, nil
}

// DefaultPageSize is used when ListQuery.Limit is zero; the documented
// maximum is 500.
const DefaultPageSize = 100

// ListInbox implements provider.InboxLister (GET /v1/inboxinvoice/list).
// OnlyNew sends isNew=true; MarkTaken sets IsNew=false. The endpoint is
// rate limited to 6 calls per 15 minutes.
func (c *Client) ListInbox(ctx context.Context, q provider.ListQuery) (*provider.Page, error) {
	const op = "list-inbox"
	kind := kindOrDefault(q.Kind)
	if err := checkKind(provider.OpListInbox, kind, provider.EInvoice); err != nil {
		return nil, err
	}
	v := url.Values{}
	page := 1
	if n, err := strconv.Atoi(q.Cursor); err == nil && n > 0 {
		page = n
	}
	size := q.Limit
	if size <= 0 {
		size = DefaultPageSize
	}
	if size > 500 {
		size = 500
	}
	v.Set("pageIndex", strconv.Itoa(page))
	v.Set("pageSize", strconv.Itoa(size))
	if q.OnlyNew {
		v.Set("isNew", "true")
	}
	const layout = "2006-01-02 15:04:05"
	if !q.From.IsZero() {
		v.Set("startDate", q.From.Format(layout))
	}
	if !q.To.IsZero() {
		v.Set("endDate", q.To.Format(layout))
	}
	var lp struct {
		PageIndex   int             `json:"pageIndex"`
		HasNextPage bool            `json:"hasNextPage"`
		Items       json.RawMessage `json:"items"`
	}
	if _, err := c.doJSON(ctx, op, http.MethodGet, "/v1/inboxinvoice/list", v, nil, &lp); err != nil {
		return nil, err
	}
	var items []inboxItem
	if raw := bytes.TrimSpace(lp.Items); len(raw) > 0 && raw[0] == '[' {
		if err := json.Unmarshal(raw, &items); err != nil {
			return nil, &provider.Error{Kind: provider.KindTransport, Provider: Name, Op: op, Message: "decode items", Err: err}
		}
	} else if len(raw) > 0 && raw[0] == '{' {
		var one inboxItem
		if err := json.Unmarshal(raw, &one); err != nil {
			return nil, &provider.Error{Kind: provider.KindTransport, Provider: Name, Op: op, Message: "decode items", Err: err}
		}
		items = append(items, one)
	}
	out := &provider.Page{Items: make([]provider.DocumentInfo, 0, len(items))}
	for _, it := range items {
		out.Items = append(out.Items, it.info())
	}
	if lp.HasNextPage {
		cur := lp.PageIndex
		if cur <= 0 {
			cur = page
		}
		out.Next = strconv.Itoa(cur + 1)
	}
	return out, nil
}

// inboxItem is InboxInvoiceGetModel.
type inboxItem struct {
	ID            string   `json:"id"`
	EnvelopeID    string   `json:"envelopeId"`
	InvoiceNumber string   `json:"invoiceNumber"`
	ExecutionDate string   `json:"executionDate"`
	Status        flexInt  `json:"status"`
	Type          flexInt  `json:"type"`
	TipType       flexInt  `json:"tipType"`
	TargetTitle   string   `json:"targetTitle"`
	TargetVknTckn string   `json:"targetVknTckn"`
	PayableAmount *float64 `json:"payableAmount"`
	Currency      string   `json:"currency"`
	CreatedDate   string   `json:"createdDate"`
	Envelope      *struct {
		Status flexInt `json:"status"`
	} `json:"envelope"`
}

// Profile and type names for the documented enum values.
var (
	profiles = map[int]string{0: "TEMELFATURA", 1: "TICARIFATURA", 2: "IHRACAT", 3: "YOLCUBERABERFATURA"}
	types    = map[int]string{1: "SATIS", 2: "IADE", 3: "ISTISNA", 4: "OZELMATRAH", 5: "TEVKIFAT", 7: "IHRACKAYITLI", 8: "SGK", 17: "KONAKLAMAVERGISI", 18: "SARJ", 19: "SARJANLIK", 20: "TEKNOLOJIDESTEK"}
)

func (it *inboxItem) info() provider.DocumentInfo {
	d := provider.DocumentInfo{
		Kind:         provider.EInvoice,
		Direction:    provider.Inbound,
		UUID:         it.ID,
		ID:           it.InvoiceNumber,
		SenderVKN:    it.TargetVknTckn,
		SenderName:   it.TargetTitle,
		Currency:     it.Currency,
		IssueDate:    datePart(it.ExecutionDate),
		CreatedAt:    parseTime(it.CreatedDate),
		ProviderCode: it.Status.raw,
		Status:       provider.StatusReceived,
	}
	if it.Type.set {
		d.Profile = profiles[it.Type.n]
	}
	if it.TipType.set {
		d.Type = types[it.TipType.n]
	}
	if it.Status.set {
		d.Status = InboxStatus(it.Status.n)
	}
	if it.PayableAmount != nil {
		d.Payable = strconv.FormatFloat(*it.PayableAmount, 'f', -1, 64)
	}
	if it.Envelope != nil && it.Envelope.Status.set {
		d.GIBCode = envelopeGIB(it.Envelope.Status.n)
	}
	return d
}

func datePart(s string) string {
	if i := strings.IndexAny(s, "T "); i > 0 {
		return s[:i]
	}
	return s
}

// Fetch implements provider.Fetcher with GET /v2/{outboxinvoice,inboxinvoice,earchive}/{id}/ubl,
// .../pdf/{isStandartXslt} and .../html/{isStandartXslt}.
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
	std := strconv.FormatBool(c.standard)
	var suffix, ct string
	switch format {
	case provider.FormatUBL:
		suffix, ct = "ubl", "application/xml"
	case provider.FormatPDF:
		suffix, ct = "pdf/"+std, "application/pdf"
	case provider.FormatHTML:
		suffix, ct = "html/"+std, "text/html"
	default:
		return nil, &provider.Error{Kind: provider.KindUnsupported, Provider: Name, Op: op, Message: "format " + string(format) + " not supported"}
	}
	var base string
	switch {
	case kind == provider.EArchive:
		base = "/v2/earchive/"
	case q.Direction == provider.Inbound:
		base = "/v2/inboxinvoice/"
	default:
		base = "/v2/outboxinvoice/"
	}
	r, err := c.do(ctx, op, http.MethodGet, base+url.PathEscape(q.UUID)+"/"+suffix, nil, nil, "")
	if err != nil {
		return nil, err
	}
	if h := r.header.Get("Content-Type"); h != "" {
		ct = h
	}
	return &provider.FetchResult{Format: format, ContentType: ct, Data: r.body}, nil
}

// Respond implements provider.Responder (POST /v1/invoiceresponse,
// Status 1 Kabul / 2 Red).
func (c *Client) Respond(ctx context.Context, r provider.Response) error {
	const op = "respond"
	if r.UUID == "" {
		return invalid(op, "UUID is required")
	}
	body := struct {
		InvoiceID string `json:"InvoiceId"`
		Status    int    `json:"Status"`
		Reason    string `json:"Reason,omitempty"`
	}{InvoiceID: r.UUID, Status: 2, Reason: r.Reason}
	if r.Accept {
		body.Status = 1
	}
	_, err := c.doJSON(ctx, op, http.MethodPost, "/v1/invoiceresponse", nil, body, nil)
	return err
}

// CancelEArchive implements provider.EArchiveCanceller
// (PUT /v1/earchive/cancelinvoice with a GUID array). Only invoices in
// status 60 can be cancelled. The API takes no reason; it is ignored.
func (c *Client) CancelEArchive(ctx context.Context, uuid, _ string) error {
	const op = "cancel-earchive"
	if uuid == "" {
		return invalid(op, "UUID is required")
	}
	_, err := c.doJSON(ctx, op, http.MethodPut, "/v1/earchive/cancelinvoice", nil, []string{uuid}, nil)
	return err
}

// MarkTaken implements provider.Acknowledger for incoming e-Fatura with
// PUT /v1/inboxinvoice/updateisnew (IsNew=false).
func (c *Client) MarkTaken(ctx context.Context, direction provider.Direction, uuids ...string) error {
	if direction != provider.Inbound {
		return provider.Unsupported(Name, provider.OpMarkTaken, provider.EInvoice)
	}
	if len(uuids) == 0 {
		return nil
	}
	type item struct {
		InvoiceID string `json:"InvoiceId"`
		IsNew     bool   `json:"IsNew"`
	}
	body := make([]item, 0, len(uuids))
	for _, u := range uuids {
		body = append(body, item{InvoiceID: u})
	}
	_, err := c.doJSON(ctx, "mark-taken", http.MethodPut, "/v1/inboxinvoice/updateisnew", nil, body, nil)
	return err
}
