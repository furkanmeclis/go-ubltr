package qnb

import (
	"context"
	"crypto/md5" //nolint:gosec // belgeHash is an MD5 checksum required by the service
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/go-ubltr/provider"
	"github.com/furkanmeclis/go-ubltr/provider/pack"
)

// Document type values. belgeGonderExt takes FATURA_UBL; listing,
// download, acknowledgement and status samples in the documentation use
// FATURA.
const (
	belgeTuruSend = "FATURA_UBL"
	belgeTuru     = "FATURA"
)

// trt is Türkiye time; the service returns local timestamps without zone.
var trt = time.FixedZone("TRT", 3*60*60)

func (c *Client) needConnector(op provider.Operation) error {
	if c.connector == nil {
		return provider.Unsupported(Name, op, provider.EInvoice)
	}
	return nil
}

// sendEInvoice calls belgeGonderExt. belgeNo is set to the ETTN, as the
// documentation recommends, so Status can query with belgeNoTipi YEREL.
func (c *Client) sendEInvoice(ctx context.Context, doc *provider.Document) (*provider.SendResult, error) {
	if err := c.needConnector(provider.OpSend); err != nil {
		return nil, err
	}
	if doc.UUID == "" {
		return nil, invalid("belgeGonderExt", "document UUID (ETTN) is required")
	}
	sum := md5.Sum(doc.XML) //nolint:gosec // checksum, not security
	p := gidenBelgeParametreleri{
		AlanEtiket:      doc.ReceiverAlias,
		BelgeHash:       strings.ToUpper(hex.EncodeToString(sum[:])),
		BelgeNo:         doc.UUID,
		BelgeTuru:       belgeTuruSend,
		BelgeVersiyon:   "1.0",
		ErpKodu:         c.cfg.ERPCode,
		GonderenEtiket:  doc.SenderAlias,
		MimeType:        "application/xml",
		VergiTcKimlikNo: c.cfg.VKN,
		Veri:            pack.Base64(doc.XML),
	}
	if doc.ID == "" {
		p.SeriNo = c.cfg.InvoiceSeries
	}
	var resp belgeGonderExtResp
	if err := c.connector.Call(ctx, "", belgeGonderExtReq{nsAttrs: connNS(), Params: p}, &resp); err != nil {
		return nil, classify(err, "belgeGonderExt")
	}
	if resp.BelgeOid == "" {
		return nil, &provider.Error{Kind: provider.KindUnknown, Provider: Name, Op: "belgeGonderExt", Message: "empty belgeOid"}
	}
	// belgeOid only means the document was queued; the result comes from
	// gidenBelgeDurumSorgulaExt.
	return &provider.SendResult{
		UUID:         doc.UUID,
		ID:           doc.ID,
		Status:       provider.StatusQueued,
		ProviderCode: resp.BelgeOid,
		Message:      "belgeOid " + resp.BelgeOid,
	}, nil
}

// outboundStatus queries gidenBelgeDurumSorgulaExt by local number (the
// ETTN, see sendEInvoice) and falls back to ETTN, which only finds
// documents in state İşlendi.
func (c *Client) outboundStatus(ctx context.Context, uuid string) (*gidenBelgeDurum, []byte, error) {
	if err := c.needConnector(provider.OpStatus); err != nil {
		return nil, nil, err
	}
	var lastErr error
	for _, tip := range []string{"YEREL", "ETTN"} {
		req := gidenBelgeDurumSorgulaExtReq{nsAttrs: connNS(), VKN: c.cfg.VKN, Params: gidenBelgeDurumParametreleri{
			BelgeNo: uuid, BelgeNoTipi: tip, BelgeTuru: belgeTuru, DonusTipiVersiyon: "5.0",
		}}
		raw, err := c.connector.Do(ctx, "", req)
		if err != nil {
			lastErr = classify(err, "gidenBelgeDurumSorgulaExt")
			if provider.KindOf(lastErr) == provider.KindNotFound {
				continue
			}
			return nil, nil, lastErr
		}
		var resp gidenBelgeDurumSorgulaExtResp
		if err := decode(raw, &resp, "gidenBelgeDurumSorgulaExt"); err != nil {
			return nil, nil, err
		}
		return &resp.Return, raw, nil
	}
	return nil, nil, lastErr
}

// mapOutbound turns durum / gonderimDurumu / gonderimCevabiKodu /
// yanitDurumu into a Status using the code table and the GİB table.
func mapOutbound(d *gidenBelgeDurum) (provider.Status, int) {
	st := lookupStatus("durum." + strconv.Itoa(d.Durum))
	if d.Durum != 3 {
		return st, 0
	}
	st = lookupStatus("gonderim." + strconv.Itoa(d.GonderimDurumu))
	gib := 0
	if d.GonderimCevabiKodu >= 1000 {
		gib = d.GonderimCevabiKodu
		if g := provider.GIBStatus(gib); g != provider.StatusUnknown {
			st = g
		}
	}
	if d.UlastiMi && (st == provider.StatusSent || st == provider.StatusProcessing || st == provider.StatusUnknown) {
		st = provider.StatusDelivered
	}
	if st == provider.StatusDelivered {
		if y := lookupStatus("yanit." + strconv.Itoa(d.YanitDurumu)); y == provider.StatusAccepted || y == provider.StatusRejected {
			st = y
		}
	}
	return st, gib
}

func lookupStatus(code string) provider.Status {
	if c, ok := Codes.Lookup(code); ok {
		return c.Status
	}
	return provider.StatusUnknown
}

func (c *Client) statusEInvoice(ctx context.Context, q provider.StatusQuery) (*provider.StatusResult, error) {
	if q.Direction == provider.Inbound {
		return nil, provider.Unsupported(Name, provider.OpStatus, provider.EInvoice)
	}
	d, raw, err := c.outboundStatus(ctx, q.UUID)
	if err != nil {
		return nil, err
	}
	st, gib := mapOutbound(d)
	msg := d.GonderimCevabiDetayi
	if d.Durum == 2 || msg == "" {
		msg = firstNonEmpty(d.Aciklama, msg)
	}
	res := &provider.StatusResult{
		UUID:         firstNonEmpty(d.Ettn, q.UUID),
		ID:           d.BelgeNo,
		Status:       st,
		GIBCode:      gib,
		ProviderCode: fmt.Sprintf("durum=%d;gonderimDurumu=%d;yanitDurumu=%d", d.Durum, d.GonderimDurumu, d.YanitDurumu),
		Message:      msg,
		UpdatedAt:    parseTime(firstNonEmpty(d.YanitTarihi, d.GonderimTarihi, d.AlimTarihi)),
		Raw:          raw,
	}
	switch d.YanitDurumu {
	case 1:
		res.ResponseCode = "RED"
	case 2:
		res.ResponseCode = "KABUL"
	}
	return res, nil
}

// inboxPageSize is the documented maximum of gelenBelgeleriListeleExt.
const inboxPageSize = 100

// ListInbox implements provider.InboxLister with gelenBelgeleriListeleExt.
// Paging uses sonAlinanBelgeSiraNumarasi; Cursor is the last belgeSiraNo
// seen ("0" or empty starts from the beginning). The service rejects date
// filters together with sonAlinanBelgeSiraNumarasi, so From / To are
// applied to faturaGelisTarihi on the client. OnlyNew sends
// belgelerAlindiMi=false.
func (c *Client) ListInbox(ctx context.Context, q provider.ListQuery) (*provider.Page, error) {
	if q.Kind != "" && q.Kind != provider.EInvoice {
		return nil, provider.Unsupported(Name, provider.OpListInbox, q.Kind)
	}
	if err := c.needConnector(provider.OpListInbox); err != nil {
		return nil, err
	}
	cursor := q.Cursor
	if cursor == "" {
		cursor = "0"
	}
	p := gelenBelgeParametreleri{
		BelgeTuru:                  belgeTuru,
		DonusTipiVersiyon:          "6.0",
		ErpKodu:                    c.cfg.ERPCode,
		SonAlinanBelgeSiraNumarasi: cursor,
		VergiTcKimlikNo:            c.cfg.VKN,
	}
	if q.OnlyNew {
		f := false
		p.BelgelerAlindiMi = &f
	}
	var resp gelenBelgeleriListeleExtResp
	if err := c.connector.Call(ctx, "", gelenBelgeleriListeleExtReq{nsAttrs: connNS(), Params: p}, &resp); err != nil {
		return nil, classify(err, "gelenBelgeleriListeleExt")
	}
	page := &provider.Page{}
	last := ""
	for _, b := range resp.Return {
		if q.Limit > 0 && len(page.Items) == q.Limit {
			page.Next = last
			return page, nil
		}
		last = b.BelgeSiraNo
		info := inboxInfo(b)
		if !q.From.IsZero() && !info.CreatedAt.IsZero() && info.CreatedAt.Before(q.From) {
			continue
		}
		if !q.To.IsZero() && !info.CreatedAt.IsZero() && info.CreatedAt.After(q.To) {
			continue
		}
		page.Items = append(page.Items, info)
	}
	if len(resp.Return) >= inboxPageSize {
		page.Next = last
	}
	return page, nil
}

func inboxInfo(b belge) provider.DocumentInfo {
	return provider.DocumentInfo{
		Kind:         provider.EInvoice,
		Direction:    provider.Inbound,
		UUID:         b.Ettn,
		ID:           b.BelgeNo,
		Profile:      firstNonEmpty(b.ProfileID, b.FaturaSenaryo),
		IssueDate:    isoDate(b.BelgeTarihi),
		SenderVKN:    b.GonderenVknTckn,
		SenderName:   firstNonEmpty(b.SaticiUnvan, b.GonderenIsim),
		ReceiverVKN:  b.AliciVkn,
		ReceiverName: b.AliciUnvan,
		Currency:     firstNonEmpty(b.OdenecekTutarDovizCinsi, b.DovizCinsi),
		Payable:      firstNonEmpty(b.OdenecekTutar, b.PayableAmount),
		Status:       provider.StatusReceived,
		ProviderCode: b.BelgeSiraNo,
		CreatedAt:    parseTime(b.FaturaGelisTarihi),
	}
}

func (c *Client) fetchEInvoice(ctx context.Context, q provider.FetchQuery, format string) ([]byte, error) {
	if err := c.needConnector(provider.OpFetch); err != nil {
		return nil, err
	}
	var resp base64Resp
	var op string
	if q.Direction == provider.Inbound {
		op = "gelenBelgeleriIndirExt"
		req := gelenBelgeleriIndirExtReq{nsAttrs: connNS(), Params: gelenBelgeParametreleri{
			BelgeFormati: format, BelgeTuru: belgeTuru, Ettn: []string{q.UUID}, VergiTcKimlikNo: c.cfg.VKN,
		}}
		if err := c.connector.Call(ctx, "", req, &resp); err != nil {
			return nil, classify(err, op)
		}
	} else {
		// gidenBelgeleriIndir takes belgeOid values; look the OID up first.
		d, _, err := c.outboundStatus(ctx, q.UUID)
		if err != nil {
			return nil, err
		}
		if d.YerelBelgeOid == "" {
			return nil, &provider.Error{Kind: provider.KindNotFound, Provider: Name, Op: "gidenBelgeleriIndir", Message: "no belgeOid for " + q.UUID}
		}
		op = "gidenBelgeleriIndir"
		req := gidenBelgeleriIndirReq{nsAttrs: connNS(), VKN: c.cfg.VKN, OIDs: []string{d.YerelBelgeOid}, BelgeTuru: belgeTuru, Format: format}
		if err := c.connector.Call(ctx, "", req, &resp); err != nil {
			return nil, classify(err, op)
		}
	}
	zipped, err := pack.FromBase64(resp.Return)
	if err != nil || len(zipped) == 0 {
		return nil, &provider.Error{Kind: provider.KindNotFound, Provider: Name, Op: op, Message: "empty document", Err: err}
	}
	return unzipDocument(zipped, format, op)
}

// unzipDocument returns the single file of a download zip, or the file
// whose extension matches the format.
func unzipDocument(zipped []byte, format, op string) ([]byte, error) {
	if !pack.IsZip(zipped) {
		return zipped, nil
	}
	files, err := pack.Unzip(zipped)
	if err != nil {
		return nil, &provider.Error{Kind: provider.KindTransport, Provider: Name, Op: op, Message: "bad zip", Err: err}
	}
	ext := map[string]string{"UBL": ".xml", "PDF": ".pdf", "HTML": ".html"}[format]
	var only []byte
	for n, b := range files {
		if strings.HasSuffix(strings.ToLower(n), ext) {
			return b, nil
		}
		only = b
	}
	if len(files) == 1 {
		return only, nil
	}
	return nil, &provider.Error{Kind: provider.KindNotFound, Provider: Name, Op: op, Message: fmt.Sprintf("no %s file in zip", format)}
}

// MarkTaken implements provider.Acknowledger with belgelerAlindi
// (incoming invoices only).
func (c *Client) MarkTaken(ctx context.Context, direction provider.Direction, uuids ...string) error {
	if direction != provider.Inbound {
		return provider.Unsupported(Name, provider.OpMarkTaken, provider.EInvoice)
	}
	if err := c.needConnector(provider.OpMarkTaken); err != nil {
		return err
	}
	if len(uuids) == 0 {
		return nil
	}
	req := belgelerAlindiReq{nsAttrs: connNS(), VKN: c.cfg.VKN, Ettn: uuids, BelgeTuru: belgeTuru}
	if err := c.connector.Call(ctx, "", req, nil); err != nil {
		return classify(err, "belgelerAlindi")
	}
	return nil
}

// CheckUser implements provider.UserChecker with efaturaKullaniciBilgisi.
func (c *Client) CheckUser(ctx context.Context, vknTckn string) (*provider.UserInfo, error) {
	if err := c.needConnector(provider.OpCheckUser); err != nil {
		return nil, err
	}
	var resp efaturaKullaniciBilgisiResp
	if err := c.connector.Call(ctx, "", efaturaKullaniciBilgisiReq{nsAttrs: connNS(), VKN: vknTckn}, &resp); err != nil {
		return nil, classify(err, "efaturaKullaniciBilgisi")
	}
	u := &provider.UserInfo{VKNTCKN: vknTckn}
	for _, k := range resp.Return {
		u.Registered = true
		if u.Title == "" {
			u.Title = k.Unvan
		}
		if k.Etiket != "" {
			u.Aliases = append(u.Aliases, provider.Alias{Name: k.Etiket, Type: "PK", Kind: provider.EInvoice, CreatedAt: parseTime(k.KayitZamani)})
		}
	}
	return u, nil
}

// Credits implements provider.CreditChecker with kontorBilgisiGetir for
// e-Fatura credits counted in units (kontorTipi FATURA, kontorBirimi
// ADET). Used is toplamAlinan minus kalan.
func (c *Client) Credits(ctx context.Context) (*provider.CreditInfo, error) {
	if err := c.needConnector(provider.OpCredits); err != nil {
		return nil, err
	}
	var resp kontorBilgisiGetirResp
	req := kontorBilgisiGetirReq{nsAttrs: connNS(), VKN: c.cfg.VKN, Tip: "FATURA", Birimi: "ADET"}
	if err := c.connector.Call(ctx, "", req, &resp); err != nil {
		return nil, classify(err, "kontorBilgisiGetir")
	}
	kalan := parseAmount(resp.Return.Kalan)
	total := parseAmount(resp.Return.ToplamAlinan)
	info := &provider.CreditInfo{Remaining: kalan}
	if total >= kalan {
		info.Used = total - kalan
	}
	return info, nil
}

func parseAmount(s string) int64 {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '.'); i >= 0 {
		s = s[:i]
	}
	n, _ := strconv.ParseInt(s, 10, 64)
	return n
}

// parseTime reads yyyyMMdd, yyyyMMddHHmmss and yyyyMMddHHmmssSSS.
func parseTime(s string) time.Time {
	s = strings.TrimSpace(s)
	for _, l := range []string{"20060102150405.000", "20060102150405", "20060102"} {
		v := s
		if l == "20060102150405.000" {
			if len(s) != 17 {
				continue
			}
			v = s[:14] + "." + s[14:]
		}
		if t, err := time.ParseInLocation(l, v, trt); err == nil {
			return t
		}
	}
	return time.Time{}
}

func isoDate(s string) string {
	if t := parseTime(s); !t.IsZero() && len(strings.TrimSpace(s)) == 8 {
		return t.Format("2006-01-02")
	}
	return s
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}
