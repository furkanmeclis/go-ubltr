package qnb

import (
	"context"

	"github.com/furkanmeclis/go-ubltr/provider"
)

// Send implements provider.Sender. e-Fatura documents go to belgeGonderExt
// (asynchronous; follow up with Status), e-Arşiv invoices to
// faturaOlusturExt (synchronous). The XML is sent unsigned; QNB signs it.
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

// Status implements provider.StatusChecker for outgoing e-Fatura and
// e-Arşiv invoices.
func (c *Client) Status(ctx context.Context, q provider.StatusQuery) (*provider.StatusResult, error) {
	switch q.Kind {
	case provider.EInvoice, "":
		return c.statusEInvoice(ctx, q)
	case provider.EArchive:
		return c.statusEArchive(ctx, q)
	}
	return nil, provider.Unsupported(Name, provider.OpStatus, q.Kind)
}

// Fetch implements provider.Fetcher. Incoming e-Fatura: gelenBelgeleriIndirExt;
// outgoing e-Fatura: gidenBelgeleriIndir (after an OID lookup); e-Arşiv:
// faturaSorgulaExt with donenBelgeFormati.
func (c *Client) Fetch(ctx context.Context, q provider.FetchQuery) (*provider.FetchResult, error) {
	f := q.Format
	if f == "" {
		f = provider.FormatUBL
	}
	name := map[provider.Format]string{provider.FormatUBL: "UBL", provider.FormatPDF: "PDF", provider.FormatHTML: "HTML"}[f]
	if name == "" {
		return nil, invalid("fetch", "unknown format "+string(f))
	}
	var (
		data []byte
		err  error
	)
	switch q.Kind {
	case provider.EInvoice, "":
		data, err = c.fetchEInvoice(ctx, q, name)
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
	return &provider.FetchResult{Format: f, ContentType: contentType(f), Data: data}, nil
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
