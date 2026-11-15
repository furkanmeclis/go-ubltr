package isnet

import (
	"context"
	"strconv"
	"strings"

	"github.com/furkanmeclis/go-ubltr/provider"
	"github.com/furkanmeclis/go-ubltr/provider/pack"
	"github.com/furkanmeclis/go-ubltr/provider/soap"
)

// call posts req to one service and returns the {Method}Result element
// after checking Result. A Result other than Success becomes an error.
func (c *Client) call(ctx context.Context, addressBook bool, method string, req any) (*node, []byte, error) {
	svc, filtered, prefix := c.invoice, c.invoiceFiltered, actionInvoice
	if addressBook {
		svc, filtered, prefix = c.addressBook, c.addressBookFiltered, actionAddressBook
	}
	if c.cfg.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.cfg.Timeout)
		defer cancel()
	}
	raw, err := svc.Do(ctx, prefix+method, req)
	if err != nil {
		return nil, raw, classifyTransport(ctx, err, method, filtered)
	}
	var resp node
	if err := soap.DecodeBody(raw, &resp); err != nil {
		return nil, raw, &provider.Error{Kind: provider.KindTransport, Provider: Name, Op: method, Message: "decode response", Raw: raw, Err: err}
	}
	res := resp.child(method + "Result")
	if res == nil {
		return nil, raw, &provider.Error{Kind: provider.KindTransport, Provider: Name, Op: method, Message: "response has no " + method + "Result", Raw: raw}
	}
	if r := res.get("Result"); !strings.EqualFold(r, "Success") {
		return nil, raw, failed(method, res.get("ErrorMessage"), raw)
	}
	return res, raw, nil
}

// paging reads ListQuery.Cursor / Limit.
func (c *Client) paging(op string, q provider.ListQuery) (pagingRequest, error) {
	p := pagingRequest{PageNumber: 1, RecordsPerPage: c.cfg.PageSize}
	if q.Limit > 0 {
		p.RecordsPerPage = q.Limit
	}
	if s := strings.TrimSpace(q.Cursor); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 {
			return p, invalid(op, "cursor must be a page number from a previous Page.Next")
		}
		p.PageNumber = n
	}
	if q.OnlyNew {
		return p, invalid(op, "İşNet has no taken flag for read-only clients; filter with From / To instead of OnlyNew")
	}
	if !q.From.IsZero() && !q.To.IsZero() && q.To.Before(q.From) {
		return p, invalid(op, "To is before From")
	}
	return p, nil
}

// next computes Page.Next from PagingResponse, falling back to a short
// page when TotalNumberOfPages is missing.
func next(res *node, req pagingRequest, got int) string {
	pr := res.child("PagingResponse")
	if total, err := strconv.Atoi(pr.get("TotalNumberOfPages")); err == nil && total > 0 {
		if req.PageNumber < total {
			return strconv.Itoa(req.PageNumber + 1)
		}
		return ""
	}
	if got >= req.RecordsPerPage && got > 0 {
		return strconv.Itoa(req.PageNumber + 1)
	}
	return ""
}

func (c *Client) vendorList() *stringList {
	if len(c.cfg.VendorNumbers) == 0 {
		return nil
	}
	return &stringList{Items: c.cfg.VendorNumbers}
}

// searchAll calls SearchAllInvoice.
func (c *Client) searchAll(ctx context.Context, r searchAllInvoiceRequest) (*node, []byte, error) {
	r.CompanyTaxCode = c.cfg.VKN
	r.IncludeMainCompany = true
	r.IncludeCompanyVendorNumberList = c.vendorList()
	return c.call(ctx, false, "SearchAllInvoice", searchAllInvoice{Request: r})
}

// searchArchive calls SearchArchiveInvoice.
func (c *Client) searchArchive(ctx context.Context, r searchArchiveInvoiceRequest) (*node, []byte, error) {
	r.CompanyTaxCode = c.cfg.VKN
	return c.call(ctx, false, "SearchArchiveInvoice", searchArchiveInvoice{Request: r})
}

// ListInbox lists incoming e-Fatura documents (SearchAllInvoice,
// InvoiceDirection=Incoming). From / To filter on InvoiceCreationDate;
// Cursor is a page number. Bodies are not requested.
func (c *Client) ListInbox(ctx context.Context, q provider.ListQuery) (*provider.Page, error) {
	if q.Kind != "" && q.Kind != provider.EInvoice {
		return nil, provider.Unsupported(Name, provider.OpListInbox, q.Kind)
	}
	return c.listInvoices(ctx, string(provider.OpListInbox), provider.Inbound, q)
}

// ListOutbox lists outgoing e-Fatura (SearchAllInvoice,
// InvoiceDirection=Outgoing) or, with Kind EArchive, e-Arşiv invoices
// (SearchArchiveInvoice).
func (c *Client) ListOutbox(ctx context.Context, q provider.ListQuery) (*provider.Page, error) {
	switch q.Kind {
	case "", provider.EInvoice:
		return c.listInvoices(ctx, string(provider.OpListOutbox), provider.Outbound, q)
	case provider.EArchive:
		return c.listArchive(ctx, string(provider.OpListOutbox), q, false)
	}
	return nil, provider.Unsupported(Name, provider.OpListOutbox, q.Kind)
}

// ListEArchiveCancelled lists e-Arşiv invoices with IsCancelled=true in
// the window, the separate query the integration note recommends because
// the cancellation indicator of list items is not confirmed. Use an
// invoice-date window wide enough to cover late cancellations; From / To
// filter on InvoiceCreationDate.
func (c *Client) ListEArchiveCancelled(ctx context.Context, q provider.ListQuery) (*provider.Page, error) {
	return c.listArchive(ctx, "list-earchive-cancelled", q, true)
}

func (c *Client) listInvoices(ctx context.Context, op string, dir provider.Direction, q provider.ListQuery) (*provider.Page, error) {
	pr, err := c.paging(op, q)
	if err != nil {
		return nil, err
	}
	res, _, err := c.searchAll(ctx, searchAllInvoiceRequest{
		InvoiceDirection:       wireDirection(dir),
		MinInvoiceCreationDate: formatTime(q.From),
		MaxInvoiceCreationDate: formatTime(q.To),
		PagingRequest:          pr,
		ResultSet:              newResultSet(content{}),
	})
	if err != nil {
		return nil, err
	}
	items := res.child("Invoices").all("Invoice")
	page := &provider.Page{Items: make([]provider.DocumentInfo, 0, len(items))}
	for _, it := range items {
		page.Items = append(page.Items, c.invoiceInfo(it, dir))
	}
	page.Next = next(res, pr, len(items))
	return page, nil
}

func (c *Client) listArchive(ctx context.Context, op string, q provider.ListQuery, cancelled bool) (*provider.Page, error) {
	pr, err := c.paging(op, q)
	if err != nil {
		return nil, err
	}
	r := searchArchiveInvoiceRequest{
		MinInvoiceCreationDate: formatTime(q.From),
		MaxInvoiceCreationDate: formatTime(q.To),
		PagingRequest:          pr,
		// The UBL is requested so ETTN and number can be read from it
		// when the list item lacks them (integration note §6.2).
		ResultSet: newResultSet(content{xml: true}),
	}
	if cancelled {
		t := true
		r.IsCancelled = &t
	}
	res, _, err := c.searchArchive(ctx, r)
	if err != nil {
		return nil, err
	}
	items := res.child("ArchiveInvoices").all("ArchiveInvoice")
	page := &provider.Page{Items: make([]provider.DocumentInfo, 0, len(items))}
	for _, it := range items {
		d := c.archiveInfo(it)
		if cancelled {
			d.Status = provider.StatusCancelled
		}
		page.Items = append(page.Items, d)
	}
	page.Next = next(res, pr, len(items))
	return page, nil
}

func wireDirection(d provider.Direction) string {
	if d == provider.Inbound {
		return dirIncoming
	}
	return dirOutgoing
}

// invoiceInfo maps an Invoice item. For incoming documents the
// counterparty in Receiver is reported as the sender, as the community
// integration cited in the integration note reads it (unconfirmed).
func (c *Client) invoiceInfo(n *node, dir provider.Direction) provider.DocumentInfo {
	st := mapInvoice(dir, n.get("Status"), n.get("DetailStatus"))
	d := provider.DocumentInfo{
		Kind:         provider.EInvoice,
		Direction:    dir,
		UUID:         n.get("ETTN", "Ettn"),
		ID:           n.get("InvoiceNumber"),
		Profile:      n.get("ScenarioType"),
		Type:         n.get("InvoiceType"),
		IssueDate:    dateOnly(n.get("InvoiceDate")),
		Currency:     n.get("CurrencyCode"),
		Payable:      n.get("TotalPayableAmount"),
		Status:       st.status,
		ProviderCode: st.raw,
		CreatedAt:    parseTime(n.get("InvoiceCreationDate")),
	}
	if st.gib >= 1000 {
		d.GIBCode = st.gib
	}
	rcv := n.child("Receiver")
	vkn, name := rcv.get("ReceiverTaxCode"), rcv.get("ReceiverName")
	if dir == provider.Inbound {
		d.SenderVKN, d.SenderName, d.ReceiverVKN = vkn, name, c.cfg.VKN
	} else {
		d.SenderVKN, d.ReceiverVKN, d.ReceiverName = c.cfg.VKN, vkn, name
	}
	return d
}

// archiveInfo maps an ArchiveInvoice item, filling ETTN, number,
// profile and type from the UBL when the item lacks them.
func (c *Client) archiveInfo(n *node) provider.DocumentInfo {
	st := mapArchive(n.get("Status"), n.get("IsCanceled", "IsCancelled"), n.get("CancellationReportSendingStatus"), n.get("ReportSendingStatus"))
	rcv := n.child("Receiver")
	d := provider.DocumentInfo{
		Kind:         provider.EArchive,
		Direction:    provider.Outbound,
		UUID:         n.get("ETTN", "Ettn"),
		ID:           n.get("InvoiceNumber"),
		Type:         n.get("InvoiceType"),
		IssueDate:    dateOnly(n.get("InvoiceDate")),
		SenderVKN:    c.cfg.VKN,
		ReceiverVKN:  rcv.get("ReceiverTaxCode"),
		ReceiverName: rcv.get("ReceiverName"),
		Currency:     n.get("CurrencyCode"),
		Payable:      n.get("TotalPayableAmount"),
		Status:       st.status,
		ProviderCode: st.raw,
		CreatedAt:    parseTime(n.get("InvoiceCreationDate")),
	}
	if ubl, err := decodeUBL(n.get("InvoiceXml")); err == nil && len(ubl) > 0 {
		doc := provider.Document{XML: ubl}
		if doc.Inspect() == nil {
			if d.UUID == "" {
				d.UUID = doc.UUID
			}
			if d.ID == "" {
				d.ID = doc.ID
			}
			if d.Type == "" {
				d.Type = doc.Type
			}
			if d.ReceiverVKN == "" {
				d.ReceiverVKN = doc.ReceiverVKN
			}
			d.Profile = doc.Profile
		}
	}
	return d
}

func dateOnly(s string) string {
	if len(s) >= 10 && s[4] == '-' && s[7] == '-' {
		return s[:10]
	}
	return s
}

// decodeUBL decodes a base64 InvoiceXml value; a zipped payload is
// unpacked.
func decodeUBL(s string) ([]byte, error) {
	if strings.TrimSpace(s) == "" {
		return nil, nil
	}
	b, err := pack.FromBase64(s)
	if err != nil {
		return nil, err
	}
	if pack.IsZip(b) {
		_, b, err = pack.UnzipOne(b)
	}
	return b, err
}

// Fetch downloads one document by ETTN with SearchAllInvoice or
// SearchArchiveInvoice (Ettn plus IsXMLIncluded / IsPDFIncluded /
// IsHtmlIncluded). For e-Fatura without Direction the outgoing box is
// searched first, then the incoming box. e-Arşiv has no HTML member.
func (c *Client) Fetch(ctx context.Context, q provider.FetchQuery) (*provider.FetchResult, error) {
	op := string(provider.OpFetch)
	q.UUID = strings.TrimSpace(q.UUID)
	if q.UUID == "" {
		return nil, invalid(op, "UUID is required")
	}
	if q.Format == "" {
		q.Format = provider.FormatUBL
	}
	var want content
	var field, ctype string
	switch q.Format {
	case provider.FormatUBL:
		want, field, ctype = content{xml: true}, "InvoiceXml", "application/xml"
	case provider.FormatPDF:
		want, field, ctype = content{pdf: true}, "InvoicePdf", "application/pdf"
	case provider.FormatHTML:
		want, field, ctype = content{html: true}, "InvoiceHtml", "text/html"
	default:
		return nil, invalid(op, "unknown format "+string(q.Format))
	}
	var item *node
	switch q.Kind {
	case "", provider.EInvoice:
		dirs := []provider.Direction{q.Direction}
		if q.Direction == "" {
			dirs = []provider.Direction{provider.Outbound, provider.Inbound}
		}
		for _, dir := range dirs {
			res, _, err := c.searchAll(ctx, searchAllInvoiceRequest{
				Ettn:             q.UUID,
				InvoiceDirection: wireDirection(dir),
				PagingRequest:    pagingRequest{PageNumber: 1, RecordsPerPage: 1},
				ResultSet:        newResultSet(want),
			})
			if err != nil {
				return nil, err
			}
			if item = res.path("Invoices", "Invoice"); item != nil {
				break
			}
		}
	case provider.EArchive:
		if q.Format == provider.FormatHTML {
			return nil, &provider.Error{Kind: provider.KindUnsupported, Provider: Name, Op: op, Message: "e-Arşiv search results carry no HTML"}
		}
		res, _, err := c.searchArchive(ctx, searchArchiveInvoiceRequest{
			Ettn:          q.UUID,
			PagingRequest: pagingRequest{PageNumber: 1, RecordsPerPage: 1},
			ResultSet:     newResultSet(want),
		})
		if err != nil {
			return nil, err
		}
		item = res.path("ArchiveInvoices", "ArchiveInvoice")
	default:
		return nil, provider.Unsupported(Name, provider.OpFetch, q.Kind)
	}
	if item == nil {
		return nil, notFound(op, q.UUID)
	}
	var data []byte
	var err error
	if q.Format == provider.FormatUBL {
		data, err = decodeUBL(item.get(field))
	} else if s := item.get(field); s != "" {
		data, err = pack.FromBase64(s)
	}
	if err != nil {
		return nil, &provider.Error{Kind: provider.KindTransport, Provider: Name, Op: op, Message: "decode " + field, Err: err}
	}
	if len(data) == 0 {
		return nil, &provider.Error{Kind: provider.KindNotFound, Provider: Name, Op: op, Message: "document found but " + field + " is empty"}
	}
	return &provider.FetchResult{Format: q.Format, ContentType: ctype, Data: data}, nil
}

// Status returns the status of one document by ETTN. e-Fatura defaults to
// the outgoing box. For e-Arşiv, when the item shows no cancellation, a
// second search with IsCancelled=true confirms it (integration note §6.3).
func (c *Client) Status(ctx context.Context, q provider.StatusQuery) (*provider.StatusResult, error) {
	op := string(provider.OpStatus)
	q.UUID = strings.TrimSpace(q.UUID)
	if q.UUID == "" {
		return nil, invalid(op, "UUID is required")
	}
	switch q.Kind {
	case "", provider.EInvoice:
		dir := q.Direction
		if dir == "" {
			dir = provider.Outbound
		}
		res, raw, err := c.searchAll(ctx, searchAllInvoiceRequest{
			Ettn:             q.UUID,
			InvoiceDirection: wireDirection(dir),
			PagingRequest:    pagingRequest{PageNumber: 1, RecordsPerPage: 1},
			ResultSet:        newResultSet(content{}),
		})
		if err != nil {
			return nil, err
		}
		it := res.path("Invoices", "Invoice")
		if it == nil {
			return nil, notFound(op, q.UUID)
		}
		st := mapInvoice(dir, it.get("Status"), it.get("DetailStatus"))
		out := &provider.StatusResult{
			UUID:         firstNonEmpty(it.get("ETTN", "Ettn"), q.UUID),
			ID:           it.get("InvoiceNumber"),
			Status:       st.status,
			ProviderCode: st.raw,
			Message:      it.get("SystemResponseDescription", "ReceiverResponseDescription", "RejectReason", "ApplicationResponseDescription"),
			ResponseCode: st.response,
			UpdatedAt:    parseTime(it.get("ApprovalDate")),
			Raw:          raw,
		}
		if st.gib >= 1000 {
			out.GIBCode = st.gib
		}
		return out, nil
	case provider.EArchive:
		req := searchArchiveInvoiceRequest{
			Ettn:          q.UUID,
			PagingRequest: pagingRequest{PageNumber: 1, RecordsPerPage: 1},
			ResultSet:     newResultSet(content{}),
		}
		res, raw, err := c.searchArchive(ctx, req)
		if err != nil {
			return nil, err
		}
		it := res.path("ArchiveInvoices", "ArchiveInvoice")
		if it == nil {
			return nil, notFound(op, q.UUID)
		}
		st := mapArchive(it.get("Status"), it.get("IsCanceled", "IsCancelled"), it.get("CancellationReportSendingStatus"), it.get("ReportSendingStatus"))
		if st.status != provider.StatusCancelled {
			t := true
			req.IsCancelled = &t
			cres, _, err := c.searchArchive(ctx, req)
			if err != nil {
				return nil, err
			}
			if cres.path("ArchiveInvoices", "ArchiveInvoice") != nil {
				st.status = provider.StatusCancelled
			}
		}
		return &provider.StatusResult{
			UUID:         firstNonEmpty(it.get("ETTN", "Ettn"), q.UUID),
			ID:           it.get("InvoiceNumber"),
			Status:       st.status,
			ProviderCode: st.raw,
			Message:      st.reportError,
			Raw:          raw,
		}, nil
	}
	return nil, provider.Unsupported(Name, provider.OpStatus, q.Kind)
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}

// CheckUser looks up a VKN/TCKN in the GİB user list with GetTaxPayer
// (AddressBookService). An empty VKN is refused because İşNet then lists
// every registered user. A "not found" answer (Result=Failed with
// "bulunamadı") returns Registered=false and no error.
func (c *Client) CheckUser(ctx context.Context, vknTckn string) (*provider.UserInfo, error) {
	op := string(provider.OpCheckUser)
	vknTckn = strings.TrimSpace(vknTckn)
	if vknTckn == "" {
		return nil, invalid(op, "VKN/TCKN is required; an empty TaxPayerTaxCode lists every registered user")
	}
	res, _, err := c.call(ctx, true, "GetTaxPayer", getTaxPayer{Request: getTaxPayerRequest{TaxPayerTaxCode: vknTckn}})
	if err != nil {
		if provider.KindOf(err) == provider.KindNotFound {
			return &provider.UserInfo{VKNTCKN: vknTckn}, nil
		}
		return nil, err
	}
	payers := res.child("TaxPayers").all("TaxPayer")
	var tp *node
	for _, p := range payers {
		if p.get("TaxPayerTaxCode") == vknTckn {
			tp = p
			break
		}
	}
	if tp == nil && len(payers) > 0 {
		tp = payers[0]
	}
	if tp == nil {
		return &provider.UserInfo{VKNTCKN: vknTckn}, nil
	}
	u := &provider.UserInfo{VKNTCKN: firstNonEmpty(tp.get("TaxPayerTaxCode"), vknTckn), Title: tp.get("TaxPayerName"), Registered: true}
	created := parseTime(tp.get("RegistrationDate"))
	for _, tag := range tp.child("InboxTagList").texts() {
		u.Aliases = append(u.Aliases, provider.Alias{Name: tag, Type: "PK", Kind: provider.EInvoice, CreatedAt: created})
	}
	for _, tag := range tp.child("OutboxTagList").texts() {
		u.Aliases = append(u.Aliases, provider.Alias{Name: tag, Type: "GB", Kind: provider.EInvoice, CreatedAt: created})
	}
	return u, nil
}

// Vendor is a company unit (CompanyVendor).
type Vendor struct {
	Number string
	Name   string
}

// Vendors lists the company's units with GetCompanyVendor. Invoices can
// be assigned to units; pass the numbers in Config.VendorNumbers so
// searches include them.
func (c *Client) Vendors(ctx context.Context) ([]Vendor, error) {
	res, _, err := c.call(ctx, false, "GetCompanyVendor", getCompanyVendor{Request: getVendorRequest{CompanyTaxCode: c.cfg.VKN}})
	if err != nil {
		return nil, err
	}
	var out []Vendor
	for _, v := range res.child("Vendors").all("CompanyVendor") {
		out = append(out, Vendor{Number: v.get("companyVendorNumber"), Name: v.get("companyVendorName")})
	}
	return out, nil
}

// DocumentViewerLink returns the GetDocumentViewerLink address of one
// document, as received. İşNet describes it as a link to download the PDF
// or XML; the community integration cited in README.md reports it comes
// back as http on port 80 while the viewer only answers https, so callers
// may need to rewrite the scheme.
func (c *Client) DocumentViewerLink(ctx context.Context, kind provider.DocumentKind, dir provider.Direction, uuid string) (string, error) {
	op := "GetDocumentViewerLink"
	uuid = strings.TrimSpace(uuid)
	if uuid == "" {
		return "", invalid(op, "UUID is required")
	}
	doc := docEInvoice
	switch kind {
	case "", provider.EInvoice:
	case provider.EArchive:
		doc, dir = docEArchive, provider.Outbound
	default:
		return "", invalid(op, "kind must be efatura or earsiv")
	}
	if dir == "" {
		dir = provider.Outbound
	}
	res, raw, err := c.call(ctx, false, op, getDocumentViewerLink{Request: documentViewerLinkRequest{
		CompanyTaxCode: c.cfg.VKN, Ettn: uuid, InvoiceDirection: wireDirection(dir), InvoiceDocumentType: doc,
	}})
	if err != nil {
		return "", err
	}
	link := res.get("DocumentViewerLink")
	if link == "" {
		return "", &provider.Error{Kind: provider.KindNotFound, Provider: Name, Op: op, Message: "empty DocumentViewerLink", Raw: raw}
	}
	return link, nil
}
