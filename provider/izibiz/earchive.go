package izibiz

import (
	"context"
	"strings"

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
