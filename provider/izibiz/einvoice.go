package izibiz

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/furkanmeclis/go-ubltr/provider"
	"github.com/furkanmeclis/go-ubltr/provider/pack"
)

// Send implements provider.Sender. e-Fatura uses EInvoiceWS SendInvoice,
// e-Arşiv uses EIArchiveWS WriteToArchiveExtended and e-İrsaliye uses
// EIrsaliyeWS SendDespatchAdvice. The XML is zipped and sent with
// COMPRESSED=Y, as in the vendor sample client.
func (p *Provider) Send(ctx context.Context, doc *provider.Document) (*provider.SendResult, error) {
	if doc == nil || len(doc.XML) == 0 {
		return nil, invalid("send", "empty document")
	}
	if err := doc.Inspect(); err != nil {
		return nil, &provider.Error{Kind: provider.KindValidation, Provider: Name, Op: "send", Err: err}
	}
	if doc.UUID == "" {
		return nil, &provider.Error{Kind: provider.KindValidation, Provider: Name, Op: "send", Message: "document has no cbc:UUID"}
	}
	switch doc.Kind {
	case provider.EInvoice:
		return p.sendInvoice(ctx, doc)
	case provider.EArchive:
		return p.sendArchive(ctx, doc)
	case provider.EDespatch:
		return p.sendDespatch(ctx, doc)
	}
	return nil, provider.Unsupported(Name, provider.OpSend, doc.Kind)
}

func zipContent(doc *provider.Document) (*content, error) {
	z, err := pack.Zip(doc.UUID+".xml", doc.XML)
	if err != nil {
		return nil, &provider.Error{Kind: provider.KindInvalidRequest, Provider: Name, Op: "send", Err: err}
	}
	return &content{Value: pack.Base64(z)}, nil
}

func (p *Provider) sendInvoice(ctx context.Context, doc *provider.Document) (*provider.SendResult, error) {
	c, err := zipContent(doc)
	if err != nil {
		return nil, err
	}
	var sender, receiver *party
	if doc.SenderAlias != "" {
		sender = &party{Alias: doc.SenderAlias}
	}
	if doc.ReceiverVKN != "" || doc.ReceiverAlias != "" {
		receiver = &party{VKN: doc.ReceiverVKN, Alias: doc.ReceiverAlias}
	}
	resp, err := invoke[sendInvoiceResponse](ctx, p, p.invoice, "send", func(h requestHeader) any {
		return sendInvoiceRequest{
			NS: nsWSDL, Header: h, Sender: sender, Receiver: receiver,
			XSLTName: p.cfg.XSLTName, Prefix: p.cfg.InvoiceSeriesPrefix,
			Invoices: []invoiceRef{{Content: c}},
		}
	}, true)
	if err != nil {
		return nil, err
	}
	id := strings.TrimSpace(resp.InvoiceID)
	if id == "" {
		id = doc.ID
	}
	return &provider.SendResult{UUID: doc.UUID, ID: id, Status: provider.StatusQueued, ProviderCode: returnCode(resp.Return)}, nil
}

func returnCode(r *requestReturn) string {
	if r == nil {
		return ""
	}
	return strings.TrimSpace(r.ReturnCode)
}

// Status implements provider.StatusChecker.
func (p *Provider) Status(ctx context.Context, q provider.StatusQuery) (*provider.StatusResult, error) {
	if q.UUID == "" {
		return nil, invalid("status", "UUID is required")
	}
	switch q.Kind {
	case provider.EInvoice, "":
		return p.invoiceStatus(ctx, q.UUID)
	case provider.EArchive:
		return p.archiveStatus(ctx, q.UUID)
	case provider.EDespatch:
		return p.despatchStatus(ctx, q.UUID)
	}
	return nil, provider.Unsupported(Name, provider.OpStatus, q.Kind)
}

func (p *Provider) invoiceStatus(ctx context.Context, uuid string) (*provider.StatusResult, error) {
	resp, err := invoke[getInvoiceStatusResponse](ctx, p, p.invoice, "status", func(h requestHeader) any {
		return getInvoiceStatusRequest{NS: nsWSDL, Header: h, Invoice: invoiceRef{UUID: uuid}}
	}, false)
	if err != nil {
		return nil, err
	}
	if resp.Status == nil {
		return nil, &provider.Error{Kind: provider.KindNotFound, Provider: Name, Op: "status", Message: "no INVOICE_STATUS in response"}
	}
	f := resp.Status.fields()
	gib := atoi(f.GIBStatusCode)
	st := documentStatus(f.StatusCode, f.Status, gib)
	code := strings.TrimSpace(f.StatusCode)
	if code == "" {
		code = strings.TrimSpace(f.Status)
	}
	return &provider.StatusResult{
		UUID:         firstNonEmpty(resp.Status.UUID, uuid),
		ID:           resp.Status.ID,
		Status:       st,
		GIBCode:      gib,
		ProviderCode: code,
		Message:      strings.TrimSpace(f.StatusDesc),
		ResponseCode: responseCode(st),
	}, nil
}

// ListInbox implements provider.InboxLister with GetInvoice
// (DIRECTION=IN, HEADER_ONLY=Y). İzibiz has no cursor: use OnlyNew and
// MarkTaken to page through new documents.
func (p *Provider) ListInbox(ctx context.Context, q provider.ListQuery) (*provider.Page, error) {
	return p.list(ctx, q, provider.Inbound, provider.OpListInbox)
}

// ListOutbox implements provider.OutboxLister. e-Fatura uses GetInvoice
// (DIRECTION=OUT); e-Arşiv uses EIArchiveWS GetEArchiveInvoiceList and
// pages by date window (see listArchive).
func (p *Provider) ListOutbox(ctx context.Context, q provider.ListQuery) (*provider.Page, error) {
	if q.Kind == provider.EArchive {
		return p.listArchive(ctx, q)
	}
	return p.list(ctx, q, provider.Outbound, provider.OpListOutbox)
}

// maxHeaderOnly is the documented LIMIT for header-only listings.
const maxHeaderOnly = 25000

func (p *Provider) list(ctx context.Context, q provider.ListQuery, dir provider.Direction, op provider.Operation) (*provider.Page, error) {
	if q.Kind != "" && q.Kind != provider.EInvoice {
		return nil, provider.Unsupported(Name, op, q.Kind)
	}
	if q.Cursor != "" {
		return nil, invalid(string(op), "cursor paging is not supported; use OnlyNew with MarkTaken")
	}
	key := invoiceSearchKey{Direction: direction(dir)}
	if q.Limit > 0 {
		key.Limit = min(q.Limit, maxHeaderOnly)
	}
	read := !q.OnlyNew
	key.ReadIncluded = &read
	if !q.From.IsZero() || !q.To.IsZero() {
		key.DateType = "CREATE"
		if !q.From.IsZero() {
			key.StartDate = q.From.Format(time.DateOnly)
		}
		if !q.To.IsZero() {
			key.EndDate = q.To.Format(time.DateOnly)
		}
	}
	resp, err := invoke[getInvoiceResponse](ctx, p, p.invoice, string(op), func(h requestHeader) any {
		return getInvoiceRequest{NS: nsWSDL, Header: h, SearchKey: key, HeaderOnly: "Y"}
	}, true)
	if err != nil {
		if provider.KindOf(err) == provider.KindNotFound {
			return &provider.Page{}, nil
		}
		return nil, err
	}
	page := &provider.Page{Items: make([]provider.DocumentInfo, 0, len(resp.Invoices))}
	for _, inv := range resp.Invoices {
		page.Items = append(page.Items, documentInfo(inv, dir))
	}
	return page, nil
}

func direction(d provider.Direction) string {
	if d == provider.Inbound {
		return "IN"
	}
	return "OUT"
}

func documentInfo(inv invoice, dir provider.Direction) provider.DocumentInfo {
	h := inv.Header
	gib := atoi(h.GIBStatusCode)
	d := provider.DocumentInfo{
		Kind:         provider.EInvoice,
		Direction:    dir,
		UUID:         inv.UUID,
		ID:           inv.ID,
		Profile:      strings.TrimSpace(h.ProfileID),
		Type:         strings.TrimSpace(h.TypeCode),
		IssueDate:    strings.TrimSpace(h.IssueDate),
		SenderVKN:    strings.TrimSpace(h.Sender),
		SenderName:   strings.TrimSpace(h.Supplier),
		ReceiverVKN:  strings.TrimSpace(h.Receiver),
		ReceiverName: strings.TrimSpace(h.Customer),
		Currency:     strings.TrimSpace(h.Payable.Currency),
		Payable:      strings.TrimSpace(h.Payable.Value),
		Status:       documentStatus(h.StatusCode, h.Status, gib),
		GIBCode:      gib,
		ProviderCode: firstNonEmpty(strings.TrimSpace(h.StatusCode), strings.TrimSpace(h.Status)),
		CreatedAt:    parseTime(h.CDate),
	}
	if strings.EqualFold(strings.TrimSpace(h.Direction), "IN") {
		d.Direction = provider.Inbound
	} else if strings.EqualFold(strings.TrimSpace(h.Direction), "OUT") {
		d.Direction = provider.Outbound
	}
	return d
}

// Fetch implements provider.Fetcher. e-Fatura UBL comes from GetInvoice,
// PDF and HTML from GetInvoiceWithType; e-Arşiv documents come from
// ReadFromArchive. Contents arrive zipped and are unzipped here.
func (p *Provider) Fetch(ctx context.Context, q provider.FetchQuery) (*provider.FetchResult, error) {
	if q.UUID == "" {
		return nil, invalid("fetch", "UUID is required")
	}
	if q.Format == "" {
		q.Format = provider.FormatUBL
	}
	switch q.Kind {
	case provider.EInvoice, "":
		return p.fetchInvoice(ctx, q)
	case provider.EArchive:
		return p.fetchArchive(ctx, q)
	}
	return nil, provider.Unsupported(Name, provider.OpFetch, q.Kind)
}

func (p *Provider) fetchInvoice(ctx context.Context, q provider.FetchQuery) (*provider.FetchResult, error) {
	read := true
	var raw string
	switch q.Format {
	case provider.FormatUBL:
		resp, err := invoke[getInvoiceResponse](ctx, p, p.invoice, "fetch", func(h requestHeader) any {
			return getInvoiceRequest{NS: nsWSDL, Header: h, HeaderOnly: "N", SearchKey: invoiceSearchKey{
				UUID: q.UUID, ReadIncluded: &read, Direction: direction(q.Direction),
			}}
		}, true)
		if err != nil {
			return nil, err
		}
		if len(resp.Invoices) == 0 {
			return nil, notFound("fetch", q.UUID)
		}
		raw = resp.Invoices[0].Content
	case provider.FormatPDF, provider.FormatHTML:
		resp, err := invoke[getInvoiceResponse](ctx, p, p.invoice, "fetch", func(h requestHeader) any {
			return getInvoiceWithTypeRequest{NS: nsWSDL, Header: h, HeaderOnly: "N", SearchKey: withTypeSearchKey{
				UUID: q.UUID, Type: strings.ToUpper(string(q.Format)), ReadIncluded: &read, Direction: direction(q.Direction),
			}}
		}, true)
		if err != nil {
			return nil, err
		}
		if len(resp.Invoices) == 0 {
			return nil, notFound("fetch", q.UUID)
		}
		raw = resp.Invoices[0].Content
	default:
		return nil, invalid("fetch", "unsupported format "+string(q.Format))
	}
	data, err := decodeContent(raw)
	if err != nil {
		return nil, &provider.Error{Kind: provider.KindTransport, Provider: Name, Op: "fetch", Message: "decode CONTENT", Err: err}
	}
	return &provider.FetchResult{Format: q.Format, ContentType: contentType(q.Format), Data: data}, nil
}

// decodeContent base64-decodes CONTENT and unzips it (repeatedly: the
// vendor sample unzips ReadFromArchive XML and HTML twice).
func decodeContent(s string) ([]byte, error) {
	b, err := pack.FromBase64(s)
	if err != nil {
		return nil, err
	}
	for i := 0; i < 3 && pack.IsZip(b); i++ {
		_, b, err = pack.UnzipOne(b)
		if err != nil {
			return nil, err
		}
	}
	if len(b) == 0 {
		return nil, errors.New("empty content")
	}
	return b, nil
}

func contentType(f provider.Format) string {
	switch f {
	case provider.FormatPDF:
		return "application/pdf"
	case provider.FormatHTML:
		return "text/html; charset=utf-8"
	}
	return "application/xml"
}

// Respond implements provider.Responder with
// SendInvoiceResponseWithServerSign (STATUS KABUL / RED). İzibiz signs the
// application response.
func (p *Provider) Respond(ctx context.Context, r provider.Response) error {
	if r.UUID == "" {
		return invalid("respond", "UUID is required")
	}
	status := "RED"
	if r.Accept {
		status = "KABUL"
	}
	var desc []string
	if r.Reason != "" {
		desc = []string{r.Reason}
	}
	_, err := invoke[serverSignResponseResponse](ctx, p, p.invoice, "respond", func(h requestHeader) any {
		return serverSignResponseRequest{NS: nsWSDL, Header: h, Status: status, Invoices: []invoiceRef{{UUID: r.UUID}}, Description: desc}
	}, false)
	return err
}

// MarkTaken implements provider.Acknowledger with MarkInvoice
// (MARK value=READ). İzibiz marks by UUID for both directions.
func (p *Provider) MarkTaken(ctx context.Context, _ provider.Direction, uuids ...string) error {
	if len(uuids) == 0 {
		return nil
	}
	refs := make([]invoiceRef, 0, len(uuids))
	for _, u := range uuids {
		refs = append(refs, invoiceRef{UUID: u})
	}
	_, err := invoke[markInvoiceResponse](ctx, p, p.invoice, "mark-taken", func(h requestHeader) any {
		return markInvoiceRequest{NS: nsWSDL, Header: h, Mark: mark{Value: "READ", Invoices: refs}}
	}, false)
	return err
}

// CheckUser implements provider.UserChecker with AuthenticationWS
// CheckUser (DOCUMENT_TYPE=INVOICE). An empty result means the VKN/TCKN is
// not an e-Fatura user.
func (p *Provider) CheckUser(ctx context.Context, vknTckn string) (*provider.UserInfo, error) {
	if vknTckn == "" {
		return nil, invalid("check-user", "VKN/TCKN is required")
	}
	resp, err := invoke[checkUserResponse](ctx, p, p.auth, "check-user", func(h requestHeader) any {
		h.Compressed = ""
		return checkUserRequest{NS: nsWSDL, Header: h, User: gibUser{Identifier: vknTckn}, DocumentType: "INVOICE"}
	}, false)
	if err != nil {
		if provider.KindOf(err) == provider.KindNotFound {
			return &provider.UserInfo{VKNTCKN: vknTckn}, nil
		}
		return nil, err
	}
	info := &provider.UserInfo{VKNTCKN: vknTckn}
	for _, u := range resp.Users {
		if strings.EqualFold(strings.TrimSpace(u.Deleted), "Y") {
			continue
		}
		if info.Title == "" {
			info.Title = strings.TrimSpace(u.Title)
		}
		info.Aliases = append(info.Aliases, provider.Alias{
			Name:      strings.TrimSpace(u.Alias),
			Type:      strings.TrimSpace(u.Unit),
			Kind:      aliasKind(u.DocumentType),
			CreatedAt: parseTime(u.AliasCreated),
		})
	}
	info.Registered = len(info.Aliases) > 0
	return info, nil
}

func aliasKind(s string) provider.DocumentKind {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "DESPATCHADVICE":
		return provider.EDespatch
	default:
		return provider.EInvoice
	}
}

// parseTime reads the date-time forms İzibiz returns.
func parseTime(s string) time.Time {
	s = strings.TrimSpace(s)
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05", time.DateOnly} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}

func invalid(op, msg string) *provider.Error {
	return &provider.Error{Kind: provider.KindInvalidRequest, Provider: Name, Op: op, Message: msg}
}

func notFound(op, uuid string) *provider.Error {
	return &provider.Error{Kind: provider.KindNotFound, Provider: Name, Op: op, Message: "document " + uuid + " not found"}
}
