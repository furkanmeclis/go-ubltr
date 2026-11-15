package sovos

import (
	"context"

	"github.com/furkanmeclis/go-ubltr/provider"
)

// Send implements provider.Sender: e-Fatura with sendUBL (DocType INVOICE,
// asynchronous; follow up with Status), e-Arşiv with sendInvoice
// (synchronous). The XML must not carry a signature; Sovos signs it.
func (c *Client) Send(ctx context.Context, doc *provider.Document) (*provider.SendResult, error) {
	if doc == nil || len(doc.XML) == 0 {
		return nil, invalid("send", "empty document")
	}
	if doc.Kind == "" || doc.UUID == "" {
		if err := doc.Inspect(); err != nil {
			return nil, &provider.Error{Kind: provider.KindValidation, Provider: Name, Op: "send", Err: err}
		}
	}
	switch doc.Kind {
	case provider.EInvoice:
		return c.sendEInvoice(ctx, doc)
	case provider.EArchive:
		return c.sendEArchive(ctx, doc)
	}
	return nil, provider.Unsupported(Name, provider.OpSend, doc.Kind)
}

// Status implements provider.StatusChecker for outgoing documents.
func (c *Client) Status(ctx context.Context, q provider.StatusQuery) (*provider.StatusResult, error) {
	switch q.Kind {
	case provider.EInvoice, "":
		return c.statusEInvoice(ctx, q)
	case provider.EArchive:
		return c.statusEArchive(ctx, q)
	}
	return nil, provider.Unsupported(Name, provider.OpStatus, q.Kind)
}

// Fetch implements provider.Fetcher.
func (c *Client) Fetch(ctx context.Context, q provider.FetchQuery) (*provider.FetchResult, error) {
	f := q.Format
	if f == "" {
		f = provider.FormatUBL
	}
	if f != provider.FormatUBL && f != provider.FormatPDF && f != provider.FormatHTML {
		return nil, invalid("fetch", "unknown format "+string(f))
	}
	var (
		data []byte
		err  error
	)
	switch q.Kind {
	case provider.EInvoice, "":
		data, err = c.fetchEInvoice(ctx, q, f)
	case provider.EArchive:
		if q.Direction == provider.Inbound {
			return nil, provider.Unsupported(Name, provider.OpFetch, q.Kind)
		}
		data, err = c.fetchEArchive(ctx, q, f)
	default:
		return nil, provider.Unsupported(Name, provider.OpFetch, q.Kind)
	}
	if err != nil {
		return nil, err
	}
	ct := "application/xml"
	switch f {
	case provider.FormatPDF:
		ct = "application/pdf"
	case provider.FormatHTML:
		ct = "text/html; charset=utf-8"
	}
	return &provider.FetchResult{Format: f, ContentType: ct, Data: data}, nil
}
