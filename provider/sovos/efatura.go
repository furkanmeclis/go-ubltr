package sovos

import (
	"context"
	"strings"
	"time"

	"github.com/furkanmeclis/go-ubltr/provider"
	"github.com/furkanmeclis/go-ubltr/provider/pack"
	"github.com/furkanmeclis/go-ubltr/provider/soap"
)

// trt is Türkiye time, used for getUBLList date windows.
var trt = time.FixedZone("TRT", 3*60*60)

func (c *Client) needEInvoice(op provider.Operation) error {
	if c.einvoice == nil {
		return provider.Unsupported(Name, op, provider.EInvoice)
	}
	return nil
}

func (c *Client) call(ctx context.Context, sc *soap.Client, prefix, op string, req, resp any) error {
	if err := sc.Call(ctx, op, req, resp); err != nil {
		return classify(err, prefix, op)
	}
	return nil
}

// sendEInvoice sends one invoice without an envelope (DocType INVOICE).
// The zip holds a single <UUID>.xml as the documentation requires; Sovos
// creates the envelope and signs the invoice.
func (c *Client) sendEInvoice(ctx context.Context, doc *provider.Document) (*provider.SendResult, error) {
	if err := c.needEInvoice(provider.OpSend); err != nil {
		return nil, err
	}
	sender := firstNonEmpty(doc.SenderAlias, c.cfg.SenderAlias)
	if sender == "" || doc.ReceiverAlias == "" {
		return nil, invalid("sendUBL", "SenderAlias (GB) and ReceiverAlias (PK) are required for single-invoice sends")
	}
	if doc.UUID == "" {
		return nil, invalid("sendUBL", "document UUID (ETTN) is required")
	}
	zipped, err := pack.Zip(doc.UUID+".xml", doc.XML)
	if err != nil {
		return nil, invalid("sendUBL", err.Error())
	}
	req := sendUBLRequest{
		VKN:                c.cfg.VKN,
		SenderIdentifier:   sender,
		ReceiverIdentifier: doc.ReceiverAlias,
		DocType:            "INVOICE",
		DocData:            pack.Base64(zipped),
	}
	var resp sendUBLResponse
	if err := c.call(ctx, c.einvoice, "ef.", "sendUBL", req, &resp); err != nil {
		return nil, err
	}
	res := &provider.SendResult{UUID: doc.UUID, ID: doc.ID, Status: provider.StatusQueued, GIBCode: 1000}
	for _, r := range resp.Response {
		if r.UUID == "" || strings.EqualFold(r.UUID, doc.UUID) {
			res.UUID = firstNonEmpty(r.UUID, doc.UUID)
			res.ID = firstNonEmpty(r.ID, doc.ID)
			res.EnvelopeUUID = r.EnvUUID
			res.ProviderCode = r.CustInvID
			break
		}
	}
	return res, nil
}

// envelopeOf finds the envelope UUID of an outgoing invoice with
// getUBLList (UUID query, DocType INVOICE, Type OUTBOUND).
func (c *Client) envelopeOf(ctx context.Context, uuid string) (ublListItem, error) {
	var resp getUBLListResponse
	req := getUBLListRequest{Identifier: c.cfg.SenderAlias, VKN: c.cfg.VKN, UUID: []string{uuid}, DocType: "INVOICE", Type: "OUTBOUND"}
	if err := c.call(ctx, c.einvoice, "ef.", "getUBLList", req, &resp); err != nil {
		return ublListItem{}, err
	}
	for _, it := range resp.UBLList {
		if strings.EqualFold(it.UUID, uuid) && it.EnvUUID != "" {
			return it, nil
		}
	}
	return ublListItem{}, &provider.Error{Kind: provider.KindNotFound, Provider: Name, Op: "getUBLList", Message: "no envelope for invoice " + uuid}
}

// statusEInvoice resolves the envelope and returns its GİB system
// response code from getEnvelopeStatus. Sovos uses 1000 while the
// envelope is processed internally and 1100 once it is sent to GİB
// without an answer yet. Commercial answers are not included (Sovos asks
// not to poll getInvResponses).
func (c *Client) statusEInvoice(ctx context.Context, q provider.StatusQuery) (*provider.StatusResult, error) {
	if err := c.needEInvoice(provider.OpStatus); err != nil {
		return nil, err
	}
	if q.Direction == provider.Inbound {
		return nil, provider.Unsupported(Name, provider.OpStatus, provider.EInvoice)
	}
	if c.cfg.SenderAlias == "" {
		return nil, invalid("getEnvelopeStatus", "SenderAlias (GB) is required")
	}
	item, err := c.envelopeOf(ctx, q.UUID)
	if err != nil {
		return nil, err
	}
	raw, err := c.einvoice.Do(ctx, "getEnvelopeStatus", getEnvelopeStatusRequest{Identifier: c.cfg.SenderAlias, VKN: c.cfg.VKN, UUID: []string{item.EnvUUID}})
	if err != nil {
		return nil, classify(err, "ef.", "getEnvelopeStatus")
	}
	var resp getEnvelopeStatusResponse
	if err := soap.DecodeBody(raw, &resp); err != nil || len(resp.Response) == 0 {
		return nil, &provider.Error{Kind: provider.KindTransport, Provider: Name, Op: "getEnvelopeStatus", Message: "no envelope status", Raw: raw, Err: err}
	}
	r := resp.Response[0]
	code := atoi(r.ResponseCode)
	res := &provider.StatusResult{
		UUID:         q.UUID,
		ID:           item.ID,
		Status:       provider.GIBStatus(code),
		GIBCode:      code,
		ProviderCode: r.ResponseCode,
		Message:      r.Description,
		Raw:          raw,
	}
	if t, err := time.Parse("2006-01-02", strings.TrimSpace(r.IssueDate)); err == nil {
		res.UpdatedAt = t
	} else if t, err := time.Parse(time.RFC3339, strings.TrimSpace(r.IssueDate)); err == nil {
		res.UpdatedAt = t
	}
	return res, nil
}

// maxWindow is the largest FromDate-ToDate span getUBLList accepts.
const maxWindow = 24 * time.Hour

// ListInbox implements provider.InboxLister with getUBLList (INVOICE,
// INBOUND, PostboxAlias). Sovos allows at most one day per call, so the
// query is split into one-day windows and Page.Next is the start of the
// next window (RFC 3339). Zero From / To mean the last 24 hours. Sovos has
// no acknowledgement flag, so OnlyNew is rejected.
func (c *Client) ListInbox(ctx context.Context, q provider.ListQuery) (*provider.Page, error) {
	return c.list(ctx, q, "INBOUND", c.cfg.PostboxAlias, provider.OpListInbox)
}

// ListOutbox implements provider.OutboxLister with getUBLList (INVOICE,
// OUTBOUND, SenderAlias); paging works like ListInbox.
func (c *Client) ListOutbox(ctx context.Context, q provider.ListQuery) (*provider.Page, error) {
	return c.list(ctx, q, "OUTBOUND", c.cfg.SenderAlias, provider.OpListOutbox)
}

func (c *Client) list(ctx context.Context, q provider.ListQuery, typ, alias string, op provider.Operation) (*provider.Page, error) {
	if q.Kind != "" && q.Kind != provider.EInvoice {
		return nil, provider.Unsupported(Name, op, q.Kind)
	}
	if err := c.needEInvoice(op); err != nil {
		return nil, err
	}
	if q.OnlyNew {
		return nil, invalid("getUBLList", "OnlyNew is not supported: Sovos has no acknowledgement flag")
	}
	if alias == "" {
		return nil, invalid("getUBLList", "PostboxAlias (inbound) / SenderAlias (outbound) is required")
	}
	to := q.To
	if to.IsZero() {
		to = time.Now()
	}
	from := q.From
	if from.IsZero() {
		from = to.Add(-maxWindow)
	}
	if q.Cursor != "" {
		t, err := time.Parse(time.RFC3339Nano, q.Cursor)
		if err != nil {
			return nil, invalid("getUBLList", "bad cursor")
		}
		from = t
	}
	end := from.Add(maxWindow)
	page := &provider.Page{}
	if end.Before(to) {
		page.Next = end.Format(time.RFC3339Nano)
	} else {
		end = to
	}
	req := getUBLListRequest{
		Identifier: alias, VKN: c.cfg.VKN, DocType: "INVOICE", Type: typ,
		FromDate: from.In(trt).Format("2006-01-02T15:04:05.000-07:00"),
		ToDate:   end.In(trt).Format("2006-01-02T15:04:05.000-07:00"),
	}
	var resp getUBLListResponse
	if err := c.call(ctx, c.einvoice, "ef.", "getUBLList", req, &resp); err != nil {
		return nil, err
	}
	dir := provider.Inbound
	if typ == "OUTBOUND" {
		dir = provider.Outbound
	}
	for _, it := range resp.UBLList {
		info := provider.DocumentInfo{Kind: provider.EInvoice, Direction: dir, UUID: it.UUID, ID: it.ID, ProviderCode: it.EnvUUID}
		if t, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(it.InsertDateTime)); err == nil {
			info.CreatedAt = t
		}
		if dir == provider.Inbound {
			info.SenderVKN, info.Status = it.VKN, provider.StatusReceived
		} else {
			info.ReceiverVKN, info.Status = it.VKN, provider.StatusUnknown
		}
		page.Items = append(page.Items, info)
	}
	if q.Limit > 0 && len(page.Items) > q.Limit {
		page.Items = page.Items[:q.Limit]
	}
	return page, nil
}

// fetchEInvoice downloads UBL with getUBL (Parameters ZIP) and PDF / HTML
// with getInvoiceView.
func (c *Client) fetchEInvoice(ctx context.Context, q provider.FetchQuery, f provider.Format) ([]byte, error) {
	if err := c.needEInvoice(provider.OpFetch); err != nil {
		return nil, err
	}
	typ, alias := "OUTBOUND", c.cfg.SenderAlias
	if q.Direction == provider.Inbound {
		typ, alias = "INBOUND", c.cfg.PostboxAlias
	}
	if alias == "" {
		return nil, invalid("fetch", "PostboxAlias (inbound) / SenderAlias (outbound) is required")
	}
	var resp docDataResponse
	op := "getUBL"
	if f == provider.FormatUBL {
		req := getUBLRequest{Identifier: alias, VKN: c.cfg.VKN, UUID: []string{q.UUID}, DocType: "INVOICE", Type: typ, Parameters: []string{"ZIP"}}
		if err := c.call(ctx, c.einvoice, "ef.", op, req, &resp); err != nil {
			return nil, err
		}
	} else {
		op = "getInvoiceView"
		req := getInvoiceViewRequest{UUID: q.UUID, Identifier: alias, VKN: c.cfg.VKN, Type: typ, DocType: strings.ToUpper(string(f))}
		if err := c.call(ctx, c.einvoice, "ef.", op, req, &resp); err != nil {
			return nil, err
		}
	}
	if len(resp.DocData) == 0 {
		return nil, &provider.Error{Kind: provider.KindNotFound, Provider: Name, Op: op, Message: "no document data"}
	}
	data, err := pack.FromBase64(resp.DocData[0])
	if err != nil {
		return nil, &provider.Error{Kind: provider.KindTransport, Provider: Name, Op: op, Message: "bad base64", Err: err}
	}
	return unzipIf(data, op)
}

func unzipIf(data []byte, op string) ([]byte, error) {
	if !pack.IsZip(data) {
		return data, nil
	}
	files, err := pack.Unzip(data)
	if err != nil {
		return nil, &provider.Error{Kind: provider.KindTransport, Provider: Name, Op: op, Message: "bad zip", Err: err}
	}
	for _, b := range files {
		return b, nil
	}
	return nil, &provider.Error{Kind: provider.KindNotFound, Provider: Name, Op: op, Message: "empty zip"}
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}
