package qnb

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"

	"github.com/furkanmeclis/go-ubltr/provider"
	"github.com/furkanmeclis/go-ubltr/provider/pack"
	"github.com/furkanmeclis/go-ubltr/provider/soap"
)

// donenBelgeFormati values of faturaOlusturExt / faturaSorgulaExt.
const (
	outUBL  = "0"
	outHTML = "2"
	outPDF  = "3"
	outNone = "9"
)

// earsivInput is the JSON carried in the input element.
type earsivInput struct {
	IslemID           string `json:"islemId,omitempty"`
	VKN               string `json:"vkn"`
	Sube              string `json:"sube,omitempty"`
	Kasa              string `json:"kasa,omitempty"`
	ErpKodu           string `json:"erpKodu,omitempty"`
	DonenBelgeFormati string `json:"donenBelgeFormati,omitempty"`
	NumaraVerilsinMi  *int   `json:"numaraVerilsinMi,omitempty"`
	FaturaSeri        string `json:"faturaSeri,omitempty"`
	FaturaUUID        string `json:"faturaUuid,omitempty"`
}

func (in earsivInput) String() string {
	b, _ := json.Marshal(in)
	return string(b)
}

func (c *Client) needEArchive(op provider.Operation) error {
	if c.earchive == nil {
		return provider.Unsupported(Name, op, provider.EArchive)
	}
	return nil
}

// sendEArchive calls faturaOlusturExt. The service is synchronous: the
// invoice is created, signed and numbered in the call.
func (c *Client) sendEArchive(ctx context.Context, doc *provider.Document) (*provider.SendResult, error) {
	if err := c.needEArchive(provider.OpSend); err != nil {
		return nil, err
	}
	if doc.UUID == "" {
		return nil, invalid("faturaOlusturExt", "document UUID (ETTN) is required")
	}
	in := earsivInput{
		IslemID:           doc.UUID,
		VKN:               c.cfg.VKN,
		Sube:              c.cfg.Branch,
		Kasa:              c.cfg.CashRegister,
		ErpKodu:           c.cfg.ERPCode,
		DonenBelgeFormati: outNone,
	}
	if doc.ID == "" {
		one := 1
		in.NumaraVerilsinMi = &one
		in.FaturaSeri = c.cfg.InvoiceSeries
	}
	req := faturaOlusturExtReq{nsAttrs: arcNS(), Input: in.String(), Fatura: earsivBelge{BelgeFormati: "UBL", BelgeIcerigi: pack.Base64(doc.XML)}}
	var resp belgeOutputWrapper
	raw, err := c.earchiveCall(ctx, "faturaOlusturExt", req, &resp)
	if err != nil {
		return nil, err
	}
	r := resp.Return
	return &provider.SendResult{
		UUID:         firstNonEmpty(r.extra("uuid"), doc.UUID),
		ID:           firstNonEmpty(r.extra("faturaNo"), doc.ID),
		Status:       provider.StatusDelivered,
		ProviderCode: r.ResultCode,
		Message:      r.ResultText,
		Raw:          raw,
	}, nil
}

// earchiveCall runs an e-Arşiv call and turns a non-AE00000 resultCode
// into an error. resp must expose the earsivServiceResult.
func (c *Client) earchiveCall(ctx context.Context, op string, req any, resp any) ([]byte, error) {
	raw, err := c.earchive.Do(ctx, "", req)
	if err != nil {
		return nil, classify(err, op)
	}
	if err := decode(raw, resp, op); err != nil {
		return nil, err
	}
	var r earsivServiceResult
	switch v := resp.(type) {
	case *belgeOutputWrapper:
		r = v.Return
	case *faturaIptalEtResp:
		r = v.Return
	}
	if err := resultError(op, r.ResultCode, r.ResultText, raw); err != nil {
		return nil, err
	}
	return raw, nil
}

// statusEArchive calls faturaSorgulaExt. A created invoice is final
// (StatusDelivered) unless the response carries iptalTarihi.
func (c *Client) statusEArchive(ctx context.Context, q provider.StatusQuery) (*provider.StatusResult, error) {
	if err := c.needEArchive(provider.OpStatus); err != nil {
		return nil, err
	}
	if q.Direction == provider.Inbound {
		return nil, provider.Unsupported(Name, provider.OpStatus, provider.EArchive)
	}
	in := earsivInput{VKN: c.cfg.VKN, FaturaUUID: q.UUID}
	var resp belgeOutputWrapper
	raw, err := c.earchiveCall(ctx, "faturaSorgulaExt", faturaSorgulaExtReq{nsAttrs: arcNS(), Input: in.String()}, &resp)
	if err != nil {
		return nil, err
	}
	r := resp.Return
	st := provider.StatusDelivered
	msg := r.ResultText
	if t := r.extra("iptalTarihi"); t != "" {
		st = provider.StatusCancelled
		msg = "iptalTarihi " + t
	}
	return &provider.StatusResult{
		UUID:         firstNonEmpty(r.extra("uuid"), q.UUID),
		ID:           r.extra("faturaNo"),
		Status:       st,
		ProviderCode: r.ResultCode,
		Message:      msg,
		Raw:          raw,
	}, nil
}

func (c *Client) fetchEArchive(ctx context.Context, q provider.FetchQuery, format provider.Format) ([]byte, error) {
	if err := c.needEArchive(provider.OpFetch); err != nil {
		return nil, err
	}
	out := map[provider.Format]string{provider.FormatUBL: outUBL, provider.FormatHTML: outHTML, provider.FormatPDF: outPDF}[format]
	in := earsivInput{VKN: c.cfg.VKN, FaturaUUID: q.UUID, DonenBelgeFormati: out}
	var resp belgeOutputWrapper
	if _, err := c.earchiveCall(ctx, "faturaSorgulaExt", faturaSorgulaExtReq{nsAttrs: arcNS(), Input: in.String()}, &resp); err != nil {
		return nil, err
	}
	data, err := pack.FromBase64(resp.Output.BelgeIcerigi)
	if err != nil || len(data) == 0 {
		return nil, &provider.Error{Kind: provider.KindNotFound, Provider: Name, Op: "faturaSorgulaExt", Message: "empty output", Err: err}
	}
	return gunzipIf(data), nil
}

// gunzipIf inflates gzip content; the documentation samples show gzip
// compressed belgeIcerigi values (H4sI...).
func gunzipIf(b []byte) []byte {
	if len(b) < 2 || b[0] != 0x1f || b[1] != 0x8b {
		return b
	}
	zr, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		return b
	}
	out, err := io.ReadAll(io.LimitReader(zr, pack.MaxUnzipSize))
	if err != nil {
		return b
	}
	return out
}

// CancelEArchive implements provider.EArchiveCanceller with
// faturaIptalEt. The service takes no reason; reason is ignored.
func (c *Client) CancelEArchive(ctx context.Context, uuid, _ string) error {
	if err := c.needEArchive(provider.OpCancelEArchive); err != nil {
		return err
	}
	in := earsivInput{VKN: c.cfg.VKN, FaturaUUID: uuid}
	var resp faturaIptalEtResp
	_, err := c.earchiveCall(ctx, "faturaIptalEt", faturaIptalEtReq{nsAttrs: arcNS(), Input: in.String()}, &resp)
	return err
}

func decode(raw []byte, resp any, op string) error {
	if err := soap.DecodeBody(raw, resp); err != nil {
		return &provider.Error{Kind: provider.KindTransport, Provider: Name, Op: op, Message: "decode response", Raw: raw, Err: err}
	}
	return nil
}
