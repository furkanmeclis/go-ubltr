package trendyol

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/go-ubltr/provider"
)

// fileInfo is the additionalInformation part (FileInfo / EInvoiceFileInfo).
type fileInfo struct {
	LocalReferenceID string `json:"localReferenceId,omitempty"`
	AutoInvoiceID    bool   `json:"autoInvoiceId"`
	Prefix           string `json:"prefix,omitempty"`
	Source           string `json:"source"`
	UserID           int64  `json:"userId"`
	CompanyID        int64  `json:"companyId"`
	TargetAlias      string `json:"targetAlias,omitempty"`
}

// createdDocument is the by-file response (fields used here).
type createdDocument struct {
	InvoiceUUID   string          `json:"invoiceUuid"`
	InvoiceID     string          `json:"invoiceId"`
	Status        json.RawMessage `json:"status"`
	GibStatusCode int             `json:"gibStatusCode"`
	GibStatus     string          `json:"gibStatus"`
	EnvelopeID    string          `json:"envelopeId"`
}

// statusCode reads the status enum, published as numeric strings.
func statusCode(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var n int
	if json.Unmarshal(raw, &n) == nil {
		return strconv.Itoa(n)
	}
	return ""
}

func mapStatus(code string) provider.Status {
	if info, ok := Codes.Lookup("status." + code); ok {
		return info.Status
	}
	return provider.StatusUnknown
}

// Send implements provider.Sender with outgoing-einvoice-by-file or
// earchive-by-file. The UBL is sent as invoiceFile; additionalInformation
// is a JSON part (the OpenAPI default encoding for an object property in
// multipart/form-data). Documents without cbc:ID use autoInvoiceId.
func (c *Client) Send(ctx context.Context, doc *provider.Document) (*provider.SendResult, error) {
	if doc == nil || len(doc.XML) == 0 {
		return nil, &provider.Error{Kind: provider.KindInvalidRequest, Provider: Name, Op: "send", Message: "empty document"}
	}
	if doc.Kind == "" || doc.UUID == "" {
		if err := doc.Inspect(); err != nil {
			return nil, &provider.Error{Kind: provider.KindValidation, Provider: Name, Op: "send", Err: err}
		}
	}
	op, path := "", ""
	switch doc.Kind {
	case provider.EInvoice:
		op, path = "createOutgoingEInvoiceByFile", "/api/invoice/documents/outgoing-einvoice-by-file"
	case provider.EArchive:
		op, path = "createEArchiveByFile", "/api/invoice/documents/earchive-by-file"
	default:
		return nil, provider.Unsupported(Name, provider.OpSend, doc.Kind)
	}
	if _, err := c.session(ctx); err != nil {
		return nil, err
	}
	user, company := c.ids()
	info := fileInfo{Source: c.cfg.Source, UserID: user, CompanyID: company, LocalReferenceID: doc.UUID}
	if doc.ID == "" {
		info.AutoInvoiceID, info.Prefix = true, c.cfg.InvoicePrefix
	}
	if doc.Kind == provider.EInvoice {
		info.TargetAlias = doc.ReceiverAlias
	}
	body, ct, err := multipartBody(firstNonEmpty(doc.UUID, "invoice")+".xml", doc.XML, info)
	if err != nil {
		return nil, &provider.Error{Kind: provider.KindInvalidRequest, Provider: Name, Op: op, Err: err}
	}
	var out createdDocument
	if err := c.call(ctx, request{op: op, method: http.MethodPost, path: path, raw: body, contentType: ct, out: &out}); err != nil {
		return nil, err
	}
	res := &provider.SendResult{
		UUID:         firstNonEmpty(out.InvoiceUUID, doc.UUID),
		ID:           firstNonEmpty(out.InvoiceID, doc.ID),
		EnvelopeUUID: out.EnvelopeID,
		GIBCode:      out.GibStatusCode,
	}
	if doc.Kind == provider.EArchive {
		res.Status, res.ProviderCode = provider.StatusDelivered, out.GibStatus
	} else {
		code := statusCode(out.Status)
		res.Status, res.ProviderCode = mapStatus(code), code
	}
	return res, nil
}

func multipartBody(name string, xmlData []byte, info fileInfo) ([]byte, string, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	h := textproto.MIMEHeader{}
	h.Set("Content-Disposition", `form-data; name="invoiceFile"; filename="`+name+`"`)
	h.Set("Content-Type", "application/xml")
	fw, err := w.CreatePart(h)
	if err != nil {
		return nil, "", err
	}
	if _, err := fw.Write(xmlData); err != nil {
		return nil, "", err
	}
	jh := textproto.MIMEHeader{}
	jh.Set("Content-Disposition", `form-data; name="additionalInformation"`)
	jh.Set("Content-Type", "application/json")
	jw, err := w.CreatePart(jh)
	if err != nil {
		return nil, "", err
	}
	if err := json.NewEncoder(jw).Encode(info); err != nil {
		return nil, "", err
	}
	if err := w.Close(); err != nil {
		return nil, "", err
	}
	return buf.Bytes(), w.FormDataContentType(), nil
}

// Status implements provider.StatusChecker. e-Fatura: the status enum and
// gibStatusCode of /outgoing-einvoice/status; e-Arşiv: gibStatus
// (REPORTED / READY_TO_BE_REPORTED), both meaning the invoice exists.
func (c *Client) Status(ctx context.Context, q provider.StatusQuery) (*provider.StatusResult, error) {
	if q.Direction == provider.Inbound {
		return nil, provider.Unsupported(Name, provider.OpStatus, q.Kind)
	}
	switch q.Kind {
	case provider.EInvoice, "":
		var out createdDocument
		if err := c.call(ctx, request{op: "getOutgoingEInvoiceStatus", method: http.MethodGet, path: "/api/invoice/documents/outgoing-einvoice/status/" + url.PathEscape(q.UUID), out: &out}); err != nil {
			return nil, err
		}
		code := statusCode(out.Status)
		st := mapStatus(code)
		if g := provider.GIBStatus(out.GibStatusCode); st == provider.StatusUnknown && g != provider.StatusUnknown {
			st = g
		}
		res := &provider.StatusResult{UUID: firstNonEmpty(out.InvoiceUUID, q.UUID), Status: st, GIBCode: out.GibStatusCode, ProviderCode: code}
		if info, ok := Codes.Lookup("status." + code); ok {
			res.Message = info.Message
		}
		switch code {
		case "105":
			res.ResponseCode = "RED"
		}
		return res, nil
	case provider.EArchive:
		var out createdDocument
		if err := c.call(ctx, request{op: "getEArchiveStatus", method: http.MethodGet, path: "/api/invoice/documents/earchive/status/" + url.PathEscape(q.UUID), out: &out}); err != nil {
			return nil, err
		}
		return &provider.StatusResult{UUID: firstNonEmpty(out.InvoiceUUID, q.UUID), Status: provider.StatusDelivered, ProviderCode: out.GibStatus, Message: out.GibStatus}, nil
	}
	return nil, provider.Unsupported(Name, provider.OpStatus, q.Kind)
}

type searchRequest struct {
	StartDate  string     `json:"startDate,omitempty"`
	EndDate    string     `json:"endDate,omitempty"`
	CompanyID  int64      `json:"companyId,omitempty"`
	Pagination pagination `json:"pagination"`
}

type pagination struct {
	Page int `json:"page"`
	Size int `json:"size"`
}

type searchResponse struct {
	PageSize    int `json:"pageSize"`
	CurrentPage int `json:"currentPage"`
	Content     []struct {
		EnvelopeID    string          `json:"envelopeId"`
		GibStatusCode int             `json:"gibStatusCode"`
		InvoiceUUID   string          `json:"invoiceUuid"`
		InvoiceID     string          `json:"invoiceId"`
		Status        json.RawMessage `json:"status"`
		Currency      string          `json:"currency"`
		PayableAmount *int64          `json:"payableAmount"`
		Scenario      string          `json:"scenario"`
		TypeCode      string          `json:"invoiceTypeCode"`
		IssuedAt      string          `json:"issuedAt"`
		CreatedAt     string          `json:"createdAt"`
		SenderName    string          `json:"senderName"`
		SenderSurname string          `json:"senderSurname"`
		SenderTitle   string          `json:"senderTitle"`
		SenderTaxID   string          `json:"senderTaxId"`
		ReplyType     string          `json:"replyType"`
	} `json:"content"`
}

// defaultPageSize is used when ListQuery.Limit is zero.
const defaultPageSize = 50

// ListInbox implements provider.InboxLister with
// incoming-einvoice/search. From / To are sent as startDate / endDate
// (dates); Cursor is the page number to request. The page index base is
// not documented, so Page.Next is currentPage+1 while pages are full.
// OnlyNew is rejected: the API has no acknowledgement call.
func (c *Client) ListInbox(ctx context.Context, q provider.ListQuery) (*provider.Page, error) {
	if q.Kind != "" && q.Kind != provider.EInvoice {
		return nil, provider.Unsupported(Name, provider.OpListInbox, q.Kind)
	}
	if q.OnlyNew {
		return nil, &provider.Error{Kind: provider.KindInvalidRequest, Provider: Name, Op: "searchIncomingEInvoice", Message: "OnlyNew is not supported"}
	}
	if _, err := c.session(ctx); err != nil {
		return nil, err
	}
	_, company := c.ids()
	size := q.Limit
	if size <= 0 {
		size = defaultPageSize
	}
	req := searchRequest{CompanyID: company, Pagination: pagination{Size: size}}
	if !q.From.IsZero() {
		req.StartDate = q.From.In(trt).Format("2006-01-02")
	}
	if !q.To.IsZero() {
		req.EndDate = q.To.In(trt).Format("2006-01-02")
	}
	if q.Cursor != "" {
		n, err := strconv.Atoi(q.Cursor)
		if err != nil || n < 0 {
			return nil, &provider.Error{Kind: provider.KindInvalidRequest, Provider: Name, Op: "searchIncomingEInvoice", Message: "bad cursor"}
		}
		req.Pagination.Page = n
	}
	var out searchResponse
	if err := c.call(ctx, request{op: "searchIncomingEInvoice", method: http.MethodPost, path: "/api/invoice/documents/incoming-einvoice/search", body: req, out: &out}); err != nil {
		return nil, err
	}
	page := &provider.Page{}
	for _, it := range out.Content {
		d := provider.DocumentInfo{
			Kind: provider.EInvoice, Direction: provider.Inbound, UUID: it.InvoiceUUID, ID: it.InvoiceID,
			Profile: it.Scenario, Type: it.TypeCode, SenderVKN: it.SenderTaxID,
			SenderName: firstNonEmpty(it.SenderTitle, strings.TrimSpace(it.SenderName+" "+it.SenderSurname)),
			Currency:   it.Currency, Status: provider.StatusReceived, GIBCode: it.GibStatusCode,
			ProviderCode: statusCode(it.Status),
		}
		if it.PayableAmount != nil {
			d.Payable = kurus(*it.PayableAmount)
		}
		switch it.ReplyType {
		case "APPROVED", "AUTO_APPROVED":
			d.Status = provider.StatusAccepted
		case "DENIED", "RETURNED":
			d.Status = provider.StatusRejected
		}
		if t, err := time.Parse(time.RFC3339Nano, it.IssuedAt); err == nil {
			d.IssueDate = t.In(trt).Format("2006-01-02")
		}
		if t, err := time.Parse(time.RFC3339Nano, it.CreatedAt); err == nil {
			d.CreatedAt = t
		}
		page.Items = append(page.Items, d)
	}
	if len(out.Content) > 0 && len(out.Content) >= size {
		page.Next = strconv.Itoa(out.CurrentPage + 1)
	}
	return page, nil
}

// kurus formats an amount in kuruş (the API's integer money unit).
func kurus(v int64) string {
	sign := ""
	if v < 0 {
		sign, v = "-", -v
	}
	return sign + strconv.FormatInt(v/100, 10) + "." + strconv.FormatInt(100+v%100, 10)[1:]
}

var trt = time.FixedZone("TRT", 3*60*60)

// Fetch implements provider.Fetcher: POST /documents/download returns a
// temporary link (text/plain), which is then downloaded without
// credentials.
func (c *Client) Fetch(ctx context.Context, q provider.FetchQuery) (*provider.FetchResult, error) {
	f := q.Format
	if f == "" {
		f = provider.FormatUBL
	}
	ext := map[provider.Format]string{provider.FormatUBL: "xml", provider.FormatPDF: "pdf", provider.FormatHTML: "html"}[f]
	if ext == "" {
		return nil, &provider.Error{Kind: provider.KindInvalidRequest, Provider: Name, Op: "fetch", Message: "unknown format " + string(f)}
	}
	var docType string
	switch {
	case (q.Kind == provider.EInvoice || q.Kind == "") && q.Direction == provider.Inbound:
		docType = "INCOMING_EINVOICE"
	case q.Kind == provider.EInvoice || q.Kind == "":
		docType = "EINVOICE"
	case q.Kind == provider.EArchive && q.Direction != provider.Inbound:
		docType = "EARCHIVE"
	default:
		return nil, provider.Unsupported(Name, provider.OpFetch, q.Kind)
	}
	if _, err := c.session(ctx); err != nil {
		return nil, err
	}
	_, company := c.ids()
	body := map[string]any{"documentType": docType, "fileExtension": ext, "invoiceUuid": q.UUID, "companyId": company}
	var link string
	if err := c.call(ctx, request{op: "getTemporaryDocumentDownloadUrl", method: http.MethodPost, path: "/api/invoice/documents/download", body: body, out: &link}); err != nil {
		return nil, err
	}
	link = strings.Trim(link, `"`)
	if link == "" {
		return nil, &provider.Error{Kind: provider.KindNotFound, Provider: Name, Op: "getTemporaryDocumentDownloadUrl", Message: "empty link"}
	}
	data, err := c.download(ctx, "download", link)
	if err != nil {
		return nil, err
	}
	ct := map[provider.Format]string{provider.FormatUBL: "application/xml", provider.FormatPDF: "application/pdf", provider.FormatHTML: "text/html; charset=utf-8"}[f]
	return &provider.FetchResult{Format: f, ContentType: ct, Data: data}, nil
}

// Respond implements provider.Responder with POST /documents/sent-reply
// (responseType APPROVED or DENIED, note = Reason).
func (c *Client) Respond(ctx context.Context, r provider.Response) error {
	if _, err := c.session(ctx); err != nil {
		return err
	}
	_, company := c.ids()
	typ := "DENIED"
	if r.Accept {
		typ = "APPROVED"
	}
	body := map[string]any{"invoiceUuid": r.UUID, "responseType": typ, "note": r.Reason, "companyId": company}
	return c.call(ctx, request{op: "replyIncomingEInvoice", method: http.MethodPost, path: "/api/invoice/documents/sent-reply", body: body})
}

// CheckUser implements provider.UserChecker with GET /taxpayers/{taxId}.
// HTTP 404 means the VKN/TCKN is not an e-Fatura user.
func (c *Client) CheckUser(ctx context.Context, vknTckn string) (*provider.UserInfo, error) {
	var out []struct {
		TaxID             string `json:"taxId"`
		Alias             string `json:"alias"`
		Title             string `json:"title"`
		PostBoxType       string `json:"postBoxType"`
		AliasType         string `json:"aliasType"`
		AliasCreationTime string `json:"aliasCreationTime"`
		DeletedAt         string `json:"deletedAt"`
	}
	err := c.call(ctx, request{op: "getTaxPayersByTaxId", method: http.MethodGet, path: "/api/invoice/taxpayers/" + url.PathEscape(vknTckn), out: &out})
	u := &provider.UserInfo{VKNTCKN: vknTckn}
	if provider.KindOf(err) == provider.KindNotFound {
		return u, nil
	}
	if err != nil {
		return nil, err
	}
	for _, a := range out {
		if a.DeletedAt != "" {
			continue
		}
		kind := provider.EInvoice
		if a.AliasType == "DESPATCH_ADVICE" {
			kind = provider.EDespatch
		}
		if kind == provider.EInvoice {
			u.Registered = true
		}
		if u.Title == "" {
			u.Title = a.Title
		}
		al := provider.Alias{Name: a.Alias, Type: a.PostBoxType, Kind: kind}
		if t, err := time.Parse(time.RFC3339Nano, a.AliasCreationTime); err == nil {
			al.CreatedAt = t
		}
		u.Aliases = append(u.Aliases, al)
	}
	return u, nil
}

// CancelEArchive implements provider.EArchiveCanceller with POST
// /documents/earchive/cancel. The API takes no reason.
func (c *Client) CancelEArchive(ctx context.Context, uuid, _ string) error {
	if _, err := c.session(ctx); err != nil {
		return err
	}
	_, company := c.ids()
	return c.call(ctx, request{op: "cancelEArchive", method: http.MethodPost, path: "/api/invoice/documents/earchive/cancel", body: map[string]any{"invoiceUuid": uuid, "companyId": company}})
}
