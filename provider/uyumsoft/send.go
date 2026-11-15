package uyumsoft

import (
	"context"
	"encoding/xml"

	"github.com/furkanmeclis/go-ubltr/provider"
)

// Invoice is one InvoiceInfo for SendInvoices.
type Invoice struct {
	// Document holds the UBL-TR Invoice (Kind, ReceiverVKN, ReceiverAlias
	// and Email are used as described on Send).
	Document *provider.Document
	// LocalDocumentID is InvoiceInfo/@LocalDocumentId, the caller's own
	// reference (returned by the outbox list as LocalDocumentId).
	LocalDocumentID string
	// ExtraInformation is InvoiceInfo/@ExtraInformation.
	ExtraInformation string
	// Scenario overrides Config.Scenario for this invoice.
	Scenario Scenario
	// Mail adds Notification/Mailing entries (e-mail delivery by the
	// integrator, mostly used for e-Arşiv).
	Mail []Mail
	// PaperDelivery marks an e-Arşiv invoice as delivered on paper
	// (EArchiveInvoiceInfo/@DeliveryType="Paper"); default Electronic.
	PaperDelivery bool
}

// Mail is a MailingInformation entry.
type Mail struct {
	To      string
	Subject string
	// Attach* select MailAttachmentInformation flags.
	AttachXML, AttachPDF, AttachHTML bool
}

// SendOutcome is the result for one invoice of SendInvoices.
type SendOutcome struct {
	Result *provider.SendResult
	Err    error
}

// Send implements provider.Sender with SendInvoice. The UBL is embedded
// as-is in InvoiceInfo/Invoice. TargetCustomer carries ReceiverVKN and
// ReceiverAlias (posta kutusu); Email becomes a Notification/Mailing
// entry. The account's sender alias is not part of InvoiceInfo, so
// SenderAlias is ignored.
func (c *Client) Send(ctx context.Context, doc *provider.Document) (*provider.SendResult, error) {
	out, err := c.SendInvoices(ctx, Invoice{Document: doc})
	if err != nil {
		return nil, err
	}
	return out[0].Result, out[0].Err
}

// SendInvoices sends several invoices in one SendInvoice call. The error
// is set when the whole call fails; per-invoice failures (reported by
// BasicIntegration in Value/@IsSucceded) are in SendOutcome.Err.
func (c *Client) SendInvoices(ctx context.Context, invs ...Invoice) ([]SendOutcome, error) {
	const op = "SendInvoice"
	if len(invs) == 0 {
		return nil, &provider.Error{Kind: provider.KindInvalidRequest, Provider: c.preset.Name, Op: op, Message: "no invoices"}
	}
	req := sendInvoiceRequest{UserInfo: c.userInfo()}
	for i := range invs {
		info, err := c.invoiceInfo(&invs[i])
		if err != nil {
			return nil, &provider.Error{Kind: provider.KindValidation, Provider: c.preset.Name, Op: op, Err: err}
		}
		if err := c.supports(invs[i].Document.Kind, provider.OpSend); err != nil {
			return nil, err
		}
		req.Invoices.Items = append(req.Invoices.Items, info)
	}
	res, raw, err := call[sendInvoiceResult](ctx, c, op, &req)
	if err != nil {
		return nil, err
	}
	out := make([]SendOutcome, len(invs))
	for i := range out {
		if i >= len(res.Value) {
			out[i].Err = &provider.Error{Kind: provider.KindUnknown, Provider: c.preset.Name, Op: op, Message: "no result for invoice", Raw: raw}
			continue
		}
		v := res.Value[i]
		if v.IsSucceded != nil && !*v.IsSucceded {
			e := messageError(c.preset.Name, op, v.Message)
			e.Raw = raw
			out[i].Err = e
			continue
		}
		r := &provider.SendResult{
			UUID:         v.ID,
			ID:           v.Number,
			Status:       provider.StatusQueued,
			ProviderCode: v.InvoiceScenario,
			Message:      v.Message,
			Raw:          raw,
		}
		if r.UUID == "" {
			r.UUID = invs[i].Document.UUID
		}
		out[i].Result = r
	}
	return out, nil
}

func (c *Client) invoiceInfo(inv *Invoice) (*invoiceInfo, error) {
	doc := inv.Document
	if doc == nil || len(doc.XML) == 0 {
		return nil, errEmptyDocument
	}
	if err := doc.Inspect(); err != nil {
		return nil, err
	}
	if doc.Kind != provider.EInvoice && doc.Kind != provider.EArchive {
		return nil, errNotInvoice
	}
	emb, err := embedInvoice(doc.XML)
	if err != nil {
		return nil, err
	}
	info := &invoiceInfo{
		LocalDocumentID:  inv.LocalDocumentID,
		ExtraInformation: inv.ExtraInformation,
		Invoice:          emb,
		CreateDateUtc:    formatLocal(c.now()),
	}
	if doc.ReceiverVKN != "" || doc.ReceiverAlias != "" {
		info.TargetCustomer = &targetCustomer{VknTckn: doc.ReceiverVKN, Alias: doc.ReceiverAlias}
	}
	sc := inv.Scenario
	if sc == ScenarioAuto {
		sc = c.cfg.Scenario
	}
	if sc == ScenarioAuto {
		sc = ScenarioEInvoice
		if doc.Kind == provider.EArchive {
			sc = ScenarioEArchive
		}
	}
	info.Scenario = string(sc)
	if doc.Kind == provider.EArchive {
		dt := "Electronic"
		if inv.PaperDelivery {
			dt = "Paper"
		}
		info.EArchive = &earchiveInfo{DeliveryType: dt}
	}
	mails := inv.Mail
	if doc.Email != "" {
		mails = append([]Mail{{To: doc.Email}}, mails...)
	}
	if len(mails) > 0 {
		info.Notification = &notification{}
		for _, m := range mails {
			mi := mailing{EnableNotification: true, To: m.To, Subject: m.Subject}
			if m.AttachXML || m.AttachPDF || m.AttachHTML {
				mi.Attachment = &mailAttachment{XML: m.AttachXML, PDF: m.AttachPDF, HTML: m.AttachHTML}
			}
			info.Notification.Mailing = append(info.Notification.Mailing, mi)
		}
	}
	return info, nil
}

// SendInvoice request (both WSDLs): userInfo (BasicIntegration only),
// invoices/InvoiceInfo*.
type sendInvoiceRequest struct {
	XMLName  xml.Name  `xml:"http://tempuri.org/ SendInvoice"`
	UserInfo *userInfo `xml:"userInfo,omitempty"`
	Invoices struct {
		Items []*invoiceInfo `xml:"InvoiceInfo"`
	} `xml:"invoices"`
}

// invoiceInfo is tns:InvoiceInfo; element order follows the WSDL
// sequence: Invoice, TargetCustomer, EArchiveInvoiceInfo, Scenario,
// Notification, CreateDateUtc.
type invoiceInfo struct {
	LocalDocumentID  string           `xml:"LocalDocumentId,attr,omitempty"`
	ExtraInformation string           `xml:"ExtraInformation,attr,omitempty"`
	Invoice          *embeddedInvoice `xml:"Invoice"`
	TargetCustomer   *targetCustomer  `xml:"TargetCustomer,omitempty"`
	EArchive         *earchiveInfo    `xml:"EArchiveInvoiceInfo,omitempty"`
	Scenario         string           `xml:"Scenario"`
	Notification     *notification    `xml:"Notification,omitempty"`
	CreateDateUtc    string           `xml:"CreateDateUtc"`
}

type targetCustomer struct {
	VknTckn string `xml:"VknTckn,attr,omitempty"`
	Alias   string `xml:"Alias,attr,omitempty"`
	Title   string `xml:"Title,attr,omitempty"`
}

type earchiveInfo struct {
	DeliveryType string `xml:"DeliveryType,attr"`
}

type notification struct {
	Mailing []mailing `xml:"Mailing"`
}

type mailing struct {
	EnableNotification bool            `xml:"EnableNotification,attr"`
	To                 string          `xml:"To,attr,omitempty"`
	Subject            string          `xml:"Subject,omitempty"`
	Attachment         *mailAttachment `xml:"Attachment,omitempty"`
}

type mailAttachment struct {
	XML                 bool `xml:"Xml,attr"`
	PDF                 bool `xml:"Pdf,attr"`
	HTML                bool `xml:"Html,attr"`
	AdditionalDocuments bool `xml:"AdditionalDocuments,attr"`
}

// sendInvoiceResult is InvoiceIdentitiesResponse (Uyumsoft) /
// DisApiCevapOfListOfInvoiceSubmissionResult (BasicIntegration).
type sendInvoiceResult struct {
	result
	Value []struct {
		ID              string `xml:"Id,attr"`
		Number          string `xml:"Number,attr"`
		InvoiceScenario string `xml:"InvoiceScenario,attr"`
		IsSucceded      *bool  `xml:"IsSucceded,attr"`
		Message         string `xml:"Message,attr"`
	} `xml:"Value"`
}

func (r *sendInvoiceResult) partial() bool { return !r.IsSucceded && len(r.Value) > 0 }
