package izibiz

import (
	"context"
	"strings"
	"time"

	"github.com/furkanmeclis/go-ubltr/provider"
)

func (p *Provider) sendArchive(ctx context.Context, doc *provider.Document) (*provider.SendResult, error) {
	c, err := zipContent(doc)
	if err != nil {
		return nil, err
	}
	props := archiveProperties{Type: p.cfg.EArchiveType, Series: p.cfg.ArchiveSeries, XSLTName: p.cfg.XSLTName}
	if doc.Email != "" {
		props.EmailFlag = "Y"
		props.Email = []string{doc.Email}
	}
	resp, err := invoke[archiveExtendedResponse](ctx, p, p.archive, "send", func(h requestHeader) any {
		return archiveExtendedRequest{NS: nsArchive, Header: h, Content: archiveExtendedContent{
			Properties: []archiveInvoiceProperties{{EArchiveFlag: "Y", Properties: props, Content: *c}},
		}}
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

func (p *Provider) archiveStatus(ctx context.Context, uuid string) (*provider.StatusResult, error) {
	resp, err := invoke[archiveStatusResponse](ctx, p, p.archive, "status", func(h requestHeader) any {
		return archiveStatusRequest{NS: nsArchive, Header: h, UUID: []string{uuid}}
	}, false)
	if err != nil {
		return nil, err
	}
	for _, inv := range resp.Invoices {
		h := inv.Header
		if h.UUID != "" && !strings.EqualFold(h.UUID, uuid) {
			continue
		}
		return &provider.StatusResult{
			UUID:         firstNonEmpty(h.UUID, uuid),
			ID:           strings.TrimSpace(h.InvoiceID),
			Status:       archiveStatus(h.Status),
			ProviderCode: strings.TrimSpace(h.Status),
			Message:      strings.TrimSpace(h.StatusDesc),
		}, nil
	}
	return nil, notFound("status", uuid)
}

// archiveProfiles are ReadFromArchive PROFILE values from the Developer
// Portal ("PDF, HTML, XML").
var archiveProfiles = map[provider.Format]string{
	provider.FormatUBL:  "XML",
	provider.FormatPDF:  "PDF",
	provider.FormatHTML: "HTML",
}

func (p *Provider) fetchArchive(ctx context.Context, q provider.FetchQuery) (*provider.FetchResult, error) {
	profile, ok := archiveProfiles[q.Format]
	if !ok {
		return nil, invalid("fetch", "unsupported format "+string(q.Format))
	}
	resp, err := invoke[archiveReadResponse](ctx, p, p.archive, "fetch", func(h requestHeader) any {
		return archiveReadRequest{NS: nsArchive, Header: h, InvoiceID: q.UUID, Direction: "OUT", Profile: profile}
	}, true)
	if err != nil {
		return nil, err
	}
	if len(resp.Invoices) == 0 {
		return nil, notFound("fetch", q.UUID)
	}
	data, err := decodeContent(resp.Invoices[0])
	if err != nil {
		return nil, &provider.Error{Kind: provider.KindTransport, Provider: Name, Op: "fetch", Message: "decode INVOICE", Err: err}
	}
	return &provider.FetchResult{Format: q.Format, ContentType: contentType(q.Format), Data: data}, nil
}

// CancelEArchive implements provider.EArchiveCanceller with
// CancelEArchiveInvoice. reason is sent as IPTAL_NOTU.
func (p *Provider) CancelEArchive(ctx context.Context, uuid, reason string) error {
	if uuid == "" {
		return invalid("cancel-earchive", "UUID is required")
	}
	_, err := invoke[archiveCancelResponse](ctx, p, p.archive, "cancel-earchive", func(h requestHeader) any {
		return archiveCancelRequest{NS: nsArchive, Header: h, Content: []archiveCancelContent{{UUID: uuid, Note: reason}}}
	}, false)
	return err
}

// archiveListWindow is the span of one e-Arşiv listing page.
// GetEArchiveInvoiceList has no cursor, so ListOutbox pages through
// [From, To] in windows of this size and returns the next window start
// as Page.Next.
const archiveListWindow = 24 * time.Hour

// maxArchiveList is the XSD bound on INVOICE in
// GetEArchiveInvoiceListResponse (maxOccurs=10000).
const maxArchiveList = 10000

// turkeyTime is the zone START_DATE / END_DATE are written in. The
// vendor sample sends them without an offset (YYYY-MM-DDTHH:mm:ss).
var turkeyTime = time.FixedZone("TRT", 3*60*60)

const archiveDateTime = "2006-01-02T15:04:05"

// listArchive implements ListOutbox for e-Arşiv with
// GetEArchiveInvoiceList (HEADER_ONLY=Y, READ_INCLUDED=Y).
//
// Without From one call is made and Next is empty. With From (To
// defaults to now) the range is split into archiveListWindow windows:
// each call lists one window and Next holds the start of the following
// one (RFC 3339). START_DATE and END_DATE are both sent, so a document
// dated exactly on a window boundary may appear on two pages. Limit
// applies per window.
func (p *Provider) listArchive(ctx context.Context, q provider.ListQuery) (*provider.Page, error) {
	const op = string(provider.OpListOutbox)
	if q.OnlyNew {
		return nil, invalid(op, "OnlyNew is not supported for e-Arşiv; page with From, To and Cursor")
	}
	req := archiveListRequest{NS: nsArchive, HeaderOnly: "Y", ReadIncluded: "Y"}
	if q.Limit > 0 {
		req.Limit = min(q.Limit, maxArchiveList)
	}
	var next string
	switch {
	case !q.From.IsZero():
		to := q.To
		if to.IsZero() {
			to = p.now()
		}
		start := q.From
		if q.Cursor != "" {
			c, err := time.Parse(time.RFC3339, q.Cursor)
			if err != nil || c.Before(q.From.Truncate(time.Second)) || !c.Before(to) {
				return nil, invalid(op, "cursor is not a window start inside [From, To)")
			}
			start = c
		}
		if !start.Before(to) {
			return nil, invalid(op, "From must be before To")
		}
		end := start.Add(archiveListWindow)
		if end.Before(to) {
			next = end.UTC().Format(time.RFC3339)
		} else {
			end = to
		}
		req.StartDate = start.In(turkeyTime).Format(archiveDateTime)
		req.EndDate = end.In(turkeyTime).Format(archiveDateTime)
	case q.Cursor != "":
		return nil, invalid(op, "cursor paging needs From")
	case !q.To.IsZero():
		req.EndDate = q.To.In(turkeyTime).Format(archiveDateTime)
	}
	resp, err := invoke[archiveListResponse](ctx, p, p.archive, op, func(h requestHeader) any {
		r := req
		r.Header = h
		return r
	}, false)
	if err != nil {
		if provider.KindOf(err) != provider.KindNotFound {
			return nil, err
		}
		resp = &archiveListResponse{}
	}
	page := &provider.Page{Items: make([]provider.DocumentInfo, 0, len(resp.Invoices)), Next: next}
	for _, inv := range resp.Invoices {
		page.Items = append(page.Items, archiveDocumentInfo(inv.Header))
	}
	return page, nil
}

// archiveDocumentInfo maps EARCHIVEINV/HEADER. The schema has no GİB
// status code or creation time for e-Arşiv listings.
func archiveDocumentInfo(h archiveListHeader) provider.DocumentInfo {
	code := strings.TrimSpace(h.StatusCode)
	st := archiveStatus(code)
	if st == provider.StatusUnknown {
		st = archiveStatus(h.Status)
	}
	return provider.DocumentInfo{
		Kind:         provider.EArchive,
		Direction:    provider.Outbound,
		UUID:         strings.TrimSpace(h.UUID),
		ID:           strings.TrimSpace(h.InvoiceID),
		Profile:      strings.TrimSpace(h.ProfileID),
		Type:         strings.TrimSpace(h.InvoiceType),
		IssueDate:    strings.TrimSpace(h.IssueDate),
		SenderVKN:    strings.TrimSpace(h.SenderIdentifier),
		SenderName:   strings.TrimSpace(h.SenderName),
		ReceiverVKN:  strings.TrimSpace(h.CustomerIdentifier),
		ReceiverName: strings.TrimSpace(h.CustomerName),
		Currency:     strings.TrimSpace(h.CurrencyCode),
		Payable:      strings.TrimSpace(h.PayableAmount),
		Status:       st,
		ProviderCode: firstNonEmpty(code, strings.TrimSpace(h.Status)),
	}
}
