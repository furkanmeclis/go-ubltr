package sovos

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/go-ubltr/provider"
	"github.com/furkanmeclis/go-ubltr/provider/pack"
	"github.com/furkanmeclis/go-ubltr/provider/soap"
)

func (c *Client) needEArchive(op provider.Operation) error {
	if c.earchive == nil {
		return provider.Unsupported(Name, op, provider.EArchive)
	}
	return nil
}

func arcNS(ns string) nsAttrs { return nsAttrs{Inv: ns} }

// sendEArchive calls sendInvoice (synchronous, one invoice). receiverID is
// required by the schema but unused by Sovos; the receiver VKN is sent
// when known.
func (c *Client) sendEArchive(ctx context.Context, doc *provider.Document) (*provider.SendResult, error) {
	if err := c.needEArchive(provider.OpSend); err != nil {
		return nil, err
	}
	if doc.UUID == "" {
		return nil, invalid("sendInvoice", "document UUID (ETTN) is required")
	}
	zipped, err := pack.Zip(doc.UUID+".xml", doc.XML)
	if err != nil {
		return nil, invalid("sendInvoice", err.Error())
	}
	req := sendInvoiceRequest{
		nsAttrs:    arcNS(nsArcInvoice),
		SenderID:   c.cfg.VKN,
		ReceiverID: firstNonEmpty(doc.ReceiverVKN, "1111111111"),
		DocType:    "XML",
		FileName:   doc.UUID + ".zip",
		Hash:       strings.ToUpper(pack.MD5Hex(zipped)),
		BinaryData: pack.Base64(zipped),
	}
	if c.cfg.Branch != "" {
		req.CustomizationParams = []customizationParam{{Name: "BRANCH", Value: c.cfg.Branch}}
	}
	raw, err := c.earchive.Do(ctx, "sendInvoice", req)
	if err != nil {
		return nil, classify(err, "ea.", "sendInvoice")
	}
	var resp sendInvoiceResponse
	if err := soap.DecodeBody(raw, &resp); err != nil {
		return nil, &provider.Error{Kind: provider.KindTransport, Provider: Name, Op: "sendInvoice", Message: "decode response", Raw: raw, Err: err}
	}
	if len(resp.Errors) > 0 || !strings.EqualFold(resp.Result, "SUCCESS") || len(resp.Success) == 0 {
		code, msg := "", resp.Detail
		if len(resp.Errors) > 0 {
			code, msg = strings.TrimSpace(resp.Errors[0].ErrorCode), resp.Errors[0].ErrorDesc
		}
		return nil, codeError("ea.", "sendInvoice", code, msg, raw)
	}
	s := resp.Success[0]
	st, stCode := detailStatus(resp.Detail)
	return &provider.SendResult{
		UUID:         firstNonEmpty(s.UUID, doc.UUID),
		ID:           firstNonEmpty(s.InvoiceNumber, doc.ID),
		Status:       st,
		ProviderCode: firstNonEmpty(stCode, s.SuccessCode),
		Message:      resp.Detail,
		Raw:          raw,
	}, nil
}

// detailStatus reads "130:Invoice is signed successfuly" style details.
func detailStatus(detail string) (provider.Status, string) {
	code, _, ok := strings.Cut(strings.TrimSpace(detail), ":")
	if !ok || atoi(code) == 0 {
		return provider.StatusProcessing, ""
	}
	return archiveStatus(code), code
}

func archiveStatus(code string) provider.Status {
	if info, ok := Codes.Lookup("eastatus." + strings.TrimSpace(code)); ok {
		return info.Status
	}
	return provider.StatusUnknown
}

func (c *Client) lookup(ctx context.Context, op, element, uuid, output string) (*invoiceLookupResponse, []byte, error) {
	req := invoiceLookup{
		XMLName:    xml.Name{Local: "inv:" + element},
		nsAttrs:    arcNS(nsArcInvoice),
		UUID:       uuid,
		VKN:        c.cfg.VKN,
		OutputType: output,
	}
	raw, err := c.earchive.Do(ctx, op, req)
	if err != nil {
		return nil, nil, classify(err, "ea.", op)
	}
	var resp invoiceLookupResponse
	if err := soap.DecodeBody(raw, &resp); err != nil {
		return nil, nil, &provider.Error{Kind: provider.KindTransport, Provider: Name, Op: op, Message: "decode response", Raw: raw, Err: err}
	}
	return &resp, raw, nil
}

// statusEArchive calls getStatus. statusCode follows the Sovos e-Arşiv
// status list (EK3): 100-126 processing, 130 signed and 150 reported are
// final (StatusDelivered), 400 / 460 cancelled.
func (c *Client) statusEArchive(ctx context.Context, q provider.StatusQuery) (*provider.StatusResult, error) {
	if err := c.needEArchive(provider.OpStatus); err != nil {
		return nil, err
	}
	if q.Direction == provider.Inbound {
		return nil, provider.Unsupported(Name, provider.OpStatus, provider.EArchive)
	}
	r, raw, err := c.lookup(ctx, "getStatus", "getStatusRequestType", q.UUID, "")
	if err != nil {
		return nil, err
	}
	return &provider.StatusResult{
		UUID:         firstNonEmpty(r.UUID, q.UUID),
		ID:           r.InvoiceNumber,
		Status:       archiveStatus(r.StatusCode),
		ProviderCode: r.StatusCode,
		Message:      r.Detail,
		Raw:          raw,
	}, nil
}

// fetchEArchive returns the signed UBL (getSignedInvoice) or the PDF /
// HTML view (getInvoiceDocument).
func (c *Client) fetchEArchive(ctx context.Context, q provider.FetchQuery, f provider.Format) ([]byte, error) {
	if err := c.needEArchive(provider.OpFetch); err != nil {
		return nil, err
	}
	op, el, out := "getInvoiceDocument", "getInvoiceDocumentRequestType", strings.ToUpper(string(f))
	if f == provider.FormatUBL {
		op, el, out = "getSignedInvoice", "getSignedInvoiceRequestType", ""
	}
	r, _, err := c.lookup(ctx, op, el, q.UUID, out)
	if err != nil {
		return nil, err
	}
	data, err := pack.FromBase64(r.BinaryData)
	if err != nil || len(data) == 0 {
		return nil, &provider.Error{Kind: provider.KindNotFound, Provider: Name, Op: op, Message: firstNonEmpty(r.Detail, "no document data"), Err: err}
	}
	return unzipIf(data, op)
}

// CancelInfo is one cancelInvoice entry.
type CancelInfo struct {
	// InvoiceID is the GİB e-Arşiv invoice number.
	InvoiceID string
	// TotalAmount is the tax-exclusive total, e.g. "206.19".
	TotalAmount string
	// CancelDate must fall in the reporting period; zero means today.
	CancelDate time.Time
	// CustInvID optionally identifies the invoice instead of InvoiceID.
	CustInvID string
}

// CancelInvoice calls cancelInvoice with explicit values.
func (c *Client) CancelInvoice(ctx context.Context, info CancelInfo) error {
	if err := c.needEArchive(provider.OpCancelEArchive); err != nil {
		return err
	}
	d := info.CancelDate
	if d.IsZero() {
		d = time.Now()
	}
	req := cancelRequest{nsAttrs: arcNS(nsArcCancel), List: []cancelInfo{{
		InvoiceID: info.InvoiceID, VKN: c.cfg.VKN, Branch: c.cfg.Branch,
		TotalAmount: info.TotalAmount, CancelDate: d.In(trt).Format("2006-01-02"), CustInvID: info.CustInvID,
	}}}
	raw, err := c.earchive.Do(ctx, "cancelInvoice", req)
	if err != nil {
		return classify(err, "ea.", "cancelInvoice")
	}
	var resp cancelResponse
	if err := soap.DecodeBody(raw, &resp); err != nil {
		return &provider.Error{Kind: provider.KindTransport, Provider: Name, Op: "cancelInvoice", Message: "decode response", Raw: raw, Err: err}
	}
	if !strings.EqualFold(resp.Result, "SUCCESS") {
		code, msg := "", ""
		if len(resp.Cancellation) > 0 {
			code, msg = resp.Cancellation[0].Code, resp.Cancellation[0].Message
		}
		return &provider.Error{Kind: provider.KindUnknown, Provider: Name, Op: "cancelInvoice", Code: code, Message: msg, Raw: raw}
	}
	return nil
}

// CancelEArchive implements provider.EArchiveCanceller. cancelInvoice
// needs the invoice number and the tax-exclusive total, so the signed
// invoice is downloaded first and cbc:ID and
// cac:LegalMonetaryTotal/cbc:TaxExclusiveAmount are read from it. The
// service has no reason field; reason is ignored.
func (c *Client) CancelEArchive(ctx context.Context, uuid, _ string) error {
	ubl, err := c.fetchEArchive(ctx, provider.FetchQuery{Kind: provider.EArchive, UUID: uuid, Format: provider.FormatUBL}, provider.FormatUBL)
	if err != nil {
		return err
	}
	id, total, err := invoiceTotals(ubl)
	if err != nil {
		return &provider.Error{Kind: provider.KindValidation, Provider: Name, Op: "cancelInvoice", Message: "cannot read invoice number / total", Err: err}
	}
	return c.CancelInvoice(ctx, CancelInfo{InvoiceID: id, TotalAmount: total})
}

// invoiceTotals reads the top-level cbc:ID and
// LegalMonetaryTotal/TaxExclusiveAmount of a UBL invoice.
func invoiceTotals(ubl []byte) (id, total string, err error) {
	dec := xml.NewDecoder(bytes.NewReader(ubl))
	var stack []string
	for {
		tok, terr := dec.Token()
		if errors.Is(terr, io.EOF) {
			break
		}
		if terr != nil {
			return "", "", terr
		}
		switch t := tok.(type) {
		case xml.StartElement:
			stack = append(stack, t.Name.Local)
			want := (len(stack) == 2 && t.Name.Local == "ID" && id == "") ||
				(len(stack) == 3 && stack[1] == "LegalMonetaryTotal" && t.Name.Local == "TaxExclusiveAmount")
			if want {
				var v string
				if err := dec.DecodeElement(&v, &t); err != nil {
					return "", "", err
				}
				stack = stack[:len(stack)-1]
				if t.Name.Local == "ID" {
					id = strings.TrimSpace(v)
				} else {
					total = strings.TrimSpace(v)
				}
			}
		case xml.EndElement:
			stack = stack[:len(stack)-1]
		}
	}
	if id == "" || total == "" {
		return "", "", errors.New("missing cbc:ID or TaxExclusiveAmount")
	}
	if _, err := strconv.ParseFloat(total, 64); err != nil {
		return "", "", err
	}
	return id, total, nil
}
