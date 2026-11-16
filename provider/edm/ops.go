package edm

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/go-ubltr/provider"
	"github.com/furkanmeclis/go-ubltr/provider/pack"
)

// Send implements provider.Sender with SendInvoice. The UBL-TR XML is sent
// as base64 CONTENT with COMPRESSED=N, as the rebasesoftware/edm-efatura
// client does; EDM signs it.
func (p *Provider) Send(ctx context.Context, doc *provider.Document) (*provider.SendResult, error) {
	if doc == nil || len(doc.XML) == 0 {
		return nil, invalid("send", "empty document")
	}
	if err := doc.Inspect(); err != nil {
		return nil, &provider.Error{Kind: provider.KindValidation, Provider: Name, Op: "send", Err: err}
	}
	if doc.Kind != provider.EInvoice {
		return nil, provider.Unsupported(Name, provider.OpSend, doc.Kind)
	}
	if doc.UUID == "" {
		return nil, &provider.Error{Kind: provider.KindValidation, Provider: Name, Op: "send", Message: "document has no cbc:UUID"}
	}
	var sender, receiver *party
	if p.cfg.VKN != "" || doc.SenderAlias != "" {
		sender = &party{VKN: p.cfg.VKN, Alias: doc.SenderAlias}
	}
	if doc.ReceiverVKN != "" || doc.ReceiverAlias != "" {
		receiver = &party{VKN: doc.ReceiverVKN, Alias: doc.ReceiverAlias}
	}
	c := &content{Value: pack.Base64(doc.XML)}
	resp, err := invoke[sendInvoiceResponse](ctx, p, "send", actionSendInvoice, func(h requestHeader) any {
		return sendInvoiceRequest{NS: nsTempuri, Header: h, Sender: sender, Receiver: receiver, Invoices: []invoiceRef{{Content: c}}}
	})
	if err != nil {
		return nil, err
	}
	res := &provider.SendResult{UUID: doc.UUID, ID: doc.ID, Status: provider.StatusQueued}
	if resp.Return != nil {
		res.ProviderCode = strings.TrimSpace(resp.Return.ReturnCode)
	}
	for _, inv := range resp.Invoices {
		if inv.UUID != "" && !strings.EqualFold(inv.UUID, doc.UUID) {
			continue
		}
		if id := strings.TrimSpace(inv.ID); id != "" {
			res.ID = id
		}
		if s := documentStatus(inv.Header.Status, atoi(inv.Header.GIBStatusCode)); s != provider.StatusUnknown {
			res.Status = s
			res.ProviderCode = normalizeWords(inv.Header.Status)
		}
		break
	}
	return res, nil
}

// Status implements provider.StatusChecker with GetInvoiceStatus.
func (p *Provider) Status(ctx context.Context, q provider.StatusQuery) (*provider.StatusResult, error) {
	if q.Kind != "" && q.Kind != provider.EInvoice {
		return nil, provider.Unsupported(Name, provider.OpStatus, q.Kind)
	}
	if q.UUID == "" {
		return nil, invalid("status", "UUID is required")
	}
	resp, err := invoke[statusResponse](ctx, p, "status", actionStatus, func(h requestHeader) any {
		return statusRequest{NS: nsTempuri, Header: h, Invoice: invoiceRef{UUID: q.UUID}}
	})
	if err != nil {
		return nil, err
	}
	if resp.Status == nil {
		return nil, &provider.Error{Kind: provider.KindNotFound, Provider: Name, Op: "status", Message: "no INVOICE_STATUS in response"}
	}
	f := resp.Status.fields()
	gib := atoi(f.GIBStatusCode)
	st := documentStatus(f.Status, gib)
	return &provider.StatusResult{
		UUID:         firstNonEmpty(resp.Status.UUID, q.UUID),
		ID:           resp.Status.ID,
		Status:       st,
		GIBCode:      gib,
		ProviderCode: normalizeWords(f.Status),
		Message:      strings.TrimSpace(f.StatusDesc),
		ResponseCode: responseCode(st),
	}, nil
}

// ListInbox implements provider.InboxLister with GetInvoice
// (DIRECTION=IN). There is no cursor and no date filter: use OnlyNew with
// MarkTaken.
func (p *Provider) ListInbox(ctx context.Context, q provider.ListQuery) (*provider.Page, error) {
	return p.list(ctx, q, provider.Inbound, provider.OpListInbox)
}

// ListOutbox implements provider.OutboxLister (DIRECTION=OUT).
func (p *Provider) ListOutbox(ctx context.Context, q provider.ListQuery) (*provider.Page, error) {
	return p.list(ctx, q, provider.Outbound, provider.OpListOutbox)
}

func (p *Provider) list(ctx context.Context, q provider.ListQuery, dir provider.Direction, op provider.Operation) (*provider.Page, error) {
	if q.Kind != "" && q.Kind != provider.EInvoice {
		return nil, provider.Unsupported(Name, op, q.Kind)
	}
	offset := 0
	if q.Cursor != "" {
		n, err := strconv.Atoi(q.Cursor)
		if err != nil || n < 0 {
			return nil, invalid(string(op), "cursor must be a non-negative OFFSET")
		}
		offset = n
	}
	read := !q.OnlyNew
	key := searchKey{Limit: q.Limit, ReadIncluded: &read, Direction: direction(dir), Offset: offset}
	// From/To filter on the creation time (CR_START_DATE / CR_END_DATE).
	// EDM refuses READ_INCLUDED=true without a date filter, so listings that
	// include read documents default to the last DefaultListWindow.
	from, to := q.From, q.To
	if read && from.IsZero() && to.IsZero() {
		to = p.now()
		from = to.Add(-DefaultListWindow)
	}
	if !from.IsZero() {
		if to.IsZero() {
			to = p.now()
		}
		if !from.Before(to) {
			return nil, invalid(string(op), "From must be before To")
		}
		key.CrStartDate = from.In(turkey).Format(listDateTimeForm)
		key.CrEndDate = to.In(turkey).Format(listDateTimeForm)
	} else if !to.IsZero() {
		key.CrEndDate = to.In(turkey).Format(listDateTimeForm)
	}
	resp, err := invoke[getInvoiceResponse](ctx, p, string(op), actionGetInvoice, func(h requestHeader) any {
		return getInvoiceRequest{NS: nsTempuri, Header: h, SearchKey: key, HeaderOnly: headerOnlyYes, ContentType: contentTypeXML}
	})
	if err != nil {
		return nil, err
	}
	page := &provider.Page{Items: make([]provider.DocumentInfo, 0, len(resp.Invoices))}
	for _, inv := range resp.Invoices {
		page.Items = append(page.Items, documentInfo(inv, dir))
	}
	if q.Limit > 0 && len(resp.Invoices) >= q.Limit {
		page.Next = strconv.Itoa(offset + len(resp.Invoices))
	}
	return page, nil
}

// DefaultListWindow is the creation-time window used when a listing
// includes read documents and no From/To is given.
const DefaultListWindow = 30 * 24 * time.Hour

// listDateTimeForm is the xs:dateTime form sent in CR_START_DATE and
// CR_END_DATE (Türkiye local time, no zone).
const listDateTimeForm = "2006-01-02T15:04:05"

var turkey = time.FixedZone("TRT", 3*60*60)

func direction(d provider.Direction) string {
	if d == provider.Inbound {
		return directionIncoming
	}
	return directionOutgoing
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
		IssueDate:    strings.TrimSpace(h.IssueDate),
		SenderVKN:    strings.TrimSpace(h.Sender),
		SenderName:   strings.TrimSpace(h.Supplier),
		ReceiverVKN:  strings.TrimSpace(h.Receiver),
		ReceiverName: strings.TrimSpace(h.Customer),
		Currency:     strings.TrimSpace(h.Payable.Currency),
		Payable:      strings.TrimSpace(h.Payable.Value),
		Status:       documentStatus(h.Status, gib),
		GIBCode:      gib,
		ProviderCode: normalizeWords(h.Status),
		CreatedAt:    parseTime(h.CDate),
	}
	switch strings.ToUpper(strings.TrimSpace(h.Direction)) {
	case directionIncoming:
		d.Direction = provider.Inbound
	case directionOutgoing:
		d.Direction = provider.Outbound
	}
	return d
}

// Fetch implements provider.Fetcher with GetInvoice and
// INVOICE_CONTENT_TYPE XML, PDF or HTML.
func (p *Provider) Fetch(ctx context.Context, q provider.FetchQuery) (*provider.FetchResult, error) {
	if q.Kind != "" && q.Kind != provider.EInvoice {
		return nil, provider.Unsupported(Name, provider.OpFetch, q.Kind)
	}
	if q.UUID == "" {
		return nil, invalid("fetch", "UUID is required")
	}
	if q.Format == "" {
		q.Format = provider.FormatUBL
	}
	ct, ok := map[provider.Format]string{provider.FormatUBL: "XML", provider.FormatPDF: "PDF", provider.FormatHTML: "HTML"}[q.Format]
	if !ok {
		return nil, invalid("fetch", "unsupported format "+string(q.Format))
	}
	read := true
	resp, err := invoke[getInvoiceResponse](ctx, p, "fetch", actionGetInvoice, func(h requestHeader) any {
		return getInvoiceRequest{NS: nsTempuri, Header: h, HeaderOnly: headerOnlyNo, ContentType: ct, SearchKey: searchKey{
			Limit: 1, UUID: q.UUID, ReadIncluded: &read, Direction: direction(q.Direction),
		}}
	})
	if err != nil {
		return nil, err
	}
	if len(resp.Invoices) == 0 || strings.TrimSpace(resp.Invoices[0].Content) == "" {
		return nil, &provider.Error{Kind: provider.KindNotFound, Provider: Name, Op: "fetch", Message: "document " + q.UUID + " not found"}
	}
	data, err := decodeContent(resp.Invoices[0].Content)
	if err != nil {
		return nil, &provider.Error{Kind: provider.KindTransport, Provider: Name, Op: "fetch", Message: "decode CONTENT", Err: err}
	}
	return &provider.FetchResult{Format: q.Format, ContentType: contentType(q.Format), Data: data}, nil
}

func decodeContent(s string) ([]byte, error) {
	b, err := pack.FromBase64(s)
	if err != nil {
		return nil, err
	}
	if pack.IsZip(b) {
		if _, b, err = pack.UnzipOne(b); err != nil {
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

// MarkTaken implements provider.Acknowledger with MarkInvoice
// (MARK value=READ).
func (p *Provider) MarkTaken(ctx context.Context, _ provider.Direction, uuids ...string) error {
	if len(uuids) == 0 {
		return nil
	}
	refs := make([]invoiceRef, 0, len(uuids))
	for _, u := range uuids {
		refs = append(refs, invoiceRef{UUID: u})
	}
	_, err := invoke[markResponse](ctx, p, "mark-taken", actionMarkInvoice, func(h requestHeader) any {
		return markRequest{NS: nsTempuri, Header: h, Mark: mark{Value: markRead, Invoices: refs}}
	})
	return err
}

// CheckUser implements provider.UserChecker with CheckUser. Aliases with
// ALIAS_REMOVAL_TIME are skipped; an empty list means not registered.
func (p *Provider) CheckUser(ctx context.Context, vknTckn string) (*provider.UserInfo, error) {
	if vknTckn == "" {
		return nil, invalid("check-user", "VKN/TCKN is required")
	}
	resp, err := invoke[checkUserResponse](ctx, p, "check-user", actionCheckUser, func(h requestHeader) any {
		return checkUserRequest{NS: nsTempuri, Header: h, User: gibUser{Identifier: vknTckn}}
	})
	if err != nil {
		return nil, err
	}
	info := &provider.UserInfo{VKNTCKN: vknTckn}
	for _, u := range resp.Users {
		if strings.TrimSpace(u.AliasRemoved) != "" {
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

// aliasKind maps DOCUMENTTYPE when it carries a known value.
func aliasKind(s string) provider.DocumentKind {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "INVOICE":
		return provider.EInvoice
	case "DESPATCHADVICE":
		return provider.EDespatch
	}
	return ""
}

func parseTime(s string) time.Time {
	s = strings.TrimSpace(s)
	for _, layout := range []string{time.RFC3339Nano, actionDateTimeForm, time.DateOnly} {
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
