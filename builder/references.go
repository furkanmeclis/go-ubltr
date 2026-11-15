package builder

import (
	"encoding/base64"
	"strings"

	"github.com/shopspring/decimal"

	"github.com/furkanmeclis/go-ubltr/codelist"
	"github.com/furkanmeclis/go-ubltr/schema/ubltr121/cac"
	"github.com/furkanmeclis/go-ubltr/schema/ubltr121/cbc"
	"github.com/furkanmeclis/go-ubltr/schema/ubltr121/invoice"
)

const (
	// ReturnDocumentTypeCode is the BillingReference/InvoiceDocumentReference
	// DocumentTypeCode GİB requires on IADE, TEVKIFATIADE, YTBIADE, and
	// YTBTEVKIFATIADE invoices.
	ReturnDocumentTypeCode = "IADE"
	// XSLTDocumentType is the AdditionalDocumentReference DocumentType GİB
	// viewers look for when picking the embedded stylesheet.
	XSLTDocumentType = "XSLT"
	// xsltMIME is the GİB MimeCodeList value used for embedded XSLT.
	xsltMIME = "application/xml"
	// maxRateFraction is the CalculationRate fraction-digit limit in GİB Schematron.
	maxRateFraction = 6
)

type billingRef struct {
	id, date, code string
}

type xsltSpec struct {
	filename string
	data     []byte
}

// BillingReference appends cac:BillingReference/cac:InvoiceDocumentReference
// with ID and IssueDate. On return invoice types (IADE, TEVKIFATIADE, YTBIADE,
// YTBTEVKIFATIADE) DocumentTypeCode is set to [ReturnDocumentTypeCode].
func (b *InvoiceBuilder) BillingReference(id, issueDate string) *InvoiceBuilder {
	return b.BillingReferenceCode(id, issueDate, "")
}

// BillingReferenceCode is [InvoiceBuilder.BillingReference] with an explicit
// DocumentTypeCode.
func (b *InvoiceBuilder) BillingReferenceCode(id, issueDate, typeCode string) *InvoiceBuilder {
	id = strings.TrimSpace(id)
	issueDate = strings.TrimSpace(issueDate)
	if id == "" || issueDate == "" {
		b.fail("billingReference", "id and issue date are required")
		return b
	}
	b.billingRefs = append(b.billingRefs, billingRef{id: id, date: issueDate, code: strings.TrimSpace(typeCode)})
	return b
}

// ReturnOf adds a billing reference to the returned invoice (its ID and IssueDate).
func (b *InvoiceBuilder) ReturnOf(orig *invoice.Invoice) *InvoiceBuilder {
	if orig == nil {
		b.fail("billingReference", "nil invoice")
		return b
	}
	return b.BillingReference(orig.ID.Value, orig.IssueDate.Value)
}

// OrderReference sets cac:OrderReference (sipariş numarası and date).
func (b *InvoiceBuilder) OrderReference(id, issueDate string) *InvoiceBuilder {
	id = strings.TrimSpace(id)
	issueDate = strings.TrimSpace(issueDate)
	if id == "" || issueDate == "" {
		b.fail("orderReference", "id and issue date are required")
		return b
	}
	b.orderRef = &cac.OrderReferenceType{
		ID:        cbcID(id),
		IssueDate: cbc.IssueDateType{Value: issueDate},
	}
	return b
}

// DespatchReference appends cac:DespatchDocumentReference (irsaliye number and date).
func (b *InvoiceBuilder) DespatchReference(id, issueDate string) *InvoiceBuilder {
	id = strings.TrimSpace(id)
	issueDate = strings.TrimSpace(issueDate)
	if id == "" || issueDate == "" {
		b.fail("despatchReference", "id and issue date are required")
		return b
	}
	b.despatchRefs = append(b.despatchRefs, cac.DocumentReferenceType{
		ID:        cbcID(id),
		IssueDate: cbc.IssueDateType{Value: issueDate},
	})
	return b
}

// AdditionalDocument appends cac:AdditionalDocumentReference without
// embedded bytes. issueDate defaults to the invoice IssueDate; typeCode and
// docType are optional. Use [InvoiceBuilder.AdditionalDocumentReference]
// with ubltr.NewAttachmentReference for embedded files.
func (b *InvoiceBuilder) AdditionalDocument(id, issueDate, typeCode, docType string) *InvoiceBuilder {
	id = strings.TrimSpace(id)
	if id == "" {
		b.fail("additionalDocument", "id is required")
		return b
	}
	ref := cac.DocumentReferenceType{
		ID:        cbcID(id),
		IssueDate: cbc.IssueDateType{Value: strings.TrimSpace(issueDate)},
	}
	if typeCode = strings.TrimSpace(typeCode); typeCode != "" {
		ref.DocumentTypeCode = &cbc.DocumentTypeCodeType{Value: typeCode}
	}
	if docType = strings.TrimSpace(docType); docType != "" {
		ref.DocumentType = &cbc.DocumentTypeType{Value: docType}
	}
	return b.AdditionalDocumentReference(ref)
}

// AdditionalDocumentReference appends a prepared cac:AdditionalDocumentReference
// (for example from ubltr.NewAttachmentReference). An empty IssueDate is
// filled with the invoice IssueDate on Build.
func (b *InvoiceBuilder) AdditionalDocumentReference(ref cac.DocumentReferenceType) *InvoiceBuilder {
	b.additionalRefs = append(b.additionalRefs, ref)
	return b
}

// XSLT embeds stylesheet as cac:AdditionalDocumentReference with DocumentType
// [XSLTDocumentType], the invoice UUID as ID, filename as DocumentDescription,
// and a base64 EmbeddedDocumentBinaryObject with mimeCode application/xml,
// which is how GİB viewers find the invoice template.
func (b *InvoiceBuilder) XSLT(filename string, stylesheet []byte) *InvoiceBuilder {
	filename = strings.TrimSpace(filename)
	if filename == "" {
		b.fail("xslt", "filename is required")
		return b
	}
	if len(stylesheet) == 0 {
		b.fail("xslt", "stylesheet is empty")
		return b
	}
	b.xslts = append(b.xslts, xsltSpec{filename: filename, data: append([]byte(nil), stylesheet...)})
	return b
}

// XSLTReference returns an AdditionalDocumentReference that embeds an XSLT
// stylesheet the way GİB viewers expect. Append it to an existing invoice's
// AdditionalDocumentReference; [InvoiceBuilder.XSLT] does this on Build.
func XSLTReference(id, issueDate, filename string, stylesheet []byte) (cac.DocumentReferenceType, error) {
	id = strings.TrimSpace(id)
	issueDate = strings.TrimSpace(issueDate)
	filename = strings.TrimSpace(filename)
	switch {
	case id == "":
		return cac.DocumentReferenceType{}, builderErr("xslt.id", "required")
	case issueDate == "":
		return cac.DocumentReferenceType{}, builderErr("xslt.issueDate", "required")
	case filename == "":
		return cac.DocumentReferenceType{}, builderErr("xslt.filename", "required")
	case len(stylesheet) == 0:
		return cac.DocumentReferenceType{}, builderErr("xslt", "stylesheet is empty")
	}
	// UBL-TR 1.2.1 BinaryObjectType allows only mimeCode and format, so the
	// filename goes to DocumentDescription instead of @filename.
	return cac.DocumentReferenceType{
		ID:                  cbcID(id),
		IssueDate:           cbc.IssueDateType{Value: issueDate},
		DocumentType:        &cbc.DocumentTypeType{Value: XSLTDocumentType},
		DocumentDescription: []cbc.DocumentDescriptionType{{Value: filename}},
		Attachment: &cac.AttachmentType{
			EmbeddedDocumentBinaryObject: &cbc.EmbeddedDocumentBinaryObjectType{
				MimeCode: xsltMIME,
				Value:    base64.StdEncoding.EncodeToString(stylesheet),
			},
		},
	}, nil
}

// ExchangeRate sets cac:PricingExchangeRate. GİB requires it when
// DocumentCurrencyCode is not TRY: source is the document currency, target
// is usually [codelist.TRY], rate is the decimal CalculationRate (at most 6
// fraction digits), and date (YYYY-MM-DD) is optional.
func (b *InvoiceBuilder) ExchangeRate(source, target, rate, date string) *InvoiceBuilder {
	source = strings.ToUpper(strings.TrimSpace(source))
	target = strings.ToUpper(strings.TrimSpace(target))
	rate = strings.TrimSpace(rate)
	if !codelist.ValidCurrency(source) {
		b.fail("exchangeRate.source", "unknown currency "+source)
		return b
	}
	if !codelist.ValidCurrency(target) {
		b.fail("exchangeRate.target", "unknown currency "+target)
		return b
	}
	d, err := decimal.NewFromString(rate)
	if err != nil || !d.IsPositive() {
		b.fail("exchangeRate.rate", "must be a positive decimal")
		return b
	}
	if -d.Exponent() > maxRateFraction {
		b.fail("exchangeRate.rate", "at most 6 fraction digits")
		return b
	}
	er := &cac.ExchangeRateType{
		SourceCurrencyCode: cbc.SourceCurrencyCodeType{Value: source},
		TargetCurrencyCode: cbc.TargetCurrencyCodeType{Value: target},
		CalculationRate:    cbc.CalculationRateType{Value: rate},
	}
	if date = strings.TrimSpace(date); date != "" {
		er.Date = &cbc.DateType{Value: date}
	}
	b.exchange = er
	return b
}

// BuyerCustomer sets cac:BuyerCustomerParty (the foreign buyer on IHRACAT,
// the tourist on YOLCUBERABERFATURA).
func (b *InvoiceBuilder) BuyerCustomer(p cac.PartyType) *InvoiceBuilder {
	b.buyer = &p
	return b
}

// BuyerCustomerBuilder builds pb and sets cac:BuyerCustomerParty. See
// [NewExportBuyerBuilder].
func (b *InvoiceBuilder) BuyerCustomerBuilder(pb *PartyBuilder) *InvoiceBuilder {
	p, err := pb.Build()
	if err != nil {
		b.failErr("buyerCustomer", err)
		return b
	}
	return b.BuyerCustomer(p)
}

// TaxRepresentative sets cac:TaxRepresentativeParty (aracı kurum on
// YOLCUBERABERFATURA, with ARACIKURUMVKN and ARACIKURUMETIKET identifications).
func (b *InvoiceBuilder) TaxRepresentative(p cac.PartyType) *InvoiceBuilder {
	b.taxRep = &p
	return b
}

// TaxRepresentativeBuilder builds pb and sets cac:TaxRepresentativeParty.
func (b *InvoiceBuilder) TaxRepresentativeBuilder(pb *PartyBuilder) *InvoiceBuilder {
	p, err := pb.Build()
	if err != nil {
		b.failErr("taxRepresentative", err)
		return b
	}
	return b.TaxRepresentative(p)
}

// AmountInWordsNote appends a cbc:Note with the payable amount in Turkish
// words ([AmountInWords]) on Build. It requires automatic totals.
func (b *InvoiceBuilder) AmountInWordsNote() *InvoiceBuilder {
	b.amountWords = true
	return b
}

func isReturnType(typ string) bool {
	switch typ {
	case codelist.InvoiceIADE, codelist.InvoiceTEVKIFATIADE, codelist.InvoiceYTBIADE, codelist.InvoiceYTBTEVKIFATIADE:
		return true
	}
	return false
}

// withholdingAllowed lists invoice types that may carry WithholdingTaxTotal.
func withholdingAllowed(typ string) bool {
	switch typ {
	case codelist.InvoiceTEVKIFAT, codelist.InvoiceYTBTEVKIFAT,
		codelist.InvoiceTEVKIFATIADE, codelist.InvoiceYTBTEVKIFATIADE,
		codelist.InvoiceIADE, codelist.InvoiceYTBIADE,
		codelist.InvoiceSGK, codelist.InvoiceSARJ, codelist.InvoiceSARJANLIK:
		return true
	}
	return false
}

func withholdingRequired(typ string) bool {
	return typ == codelist.InvoiceTEVKIFAT || typ == codelist.InvoiceYTBTEVKIFAT
}

func (b *InvoiceBuilder) billingReferences() []cac.BillingReferenceType {
	out := make([]cac.BillingReferenceType, 0, len(b.billingRefs))
	for _, r := range b.billingRefs {
		code := r.code
		if code == "" && isReturnType(b.typ) {
			code = ReturnDocumentTypeCode
		}
		ref := &cac.DocumentReferenceType{
			ID:        cbcID(r.id),
			IssueDate: cbc.IssueDateType{Value: r.date},
		}
		if code != "" {
			ref.DocumentTypeCode = &cbc.DocumentTypeCodeType{Value: code}
		}
		out = append(out, cac.BillingReferenceType{InvoiceDocumentReference: ref})
	}
	return out
}

func (b *InvoiceBuilder) additionalReferences(uuid string) ([]cac.DocumentReferenceType, error) {
	out := make([]cac.DocumentReferenceType, 0, len(b.additionalRefs)+len(b.xslts))
	for _, r := range b.additionalRefs {
		if r.IssueDate.Value == "" {
			r.IssueDate = cbc.IssueDateType{Value: b.issueDate}
		}
		out = append(out, r)
	}
	for _, x := range b.xslts {
		ref, err := XSLTReference(uuid, b.issueDate, x.filename, x.data)
		if err != nil {
			return nil, err
		}
		out = append(out, ref)
	}
	return out, nil
}

// checkRules applies the GİB rules that can be decided before XML: return
// references, withholding versus type, exchange rate, and IHRACAT fields.
func (b *InvoiceBuilder) checkRules() error {
	if isReturnType(b.typ) {
		if len(b.billingRefs) == 0 {
			return builderErr("billingReference", "required for invoice type "+b.typ)
		}
		for _, r := range b.billingRefs {
			if len([]rune(r.id)) != 16 {
				return builderErr("billingReference", "returned invoice ID "+r.id+" must be 16 characters")
			}
		}
	}

	hasWH := false
	for _, line := range b.lines {
		if len(line.WithholdingTaxTotal) > 0 {
			hasWH = true
			break
		}
	}
	if hasWH && !withholdingAllowed(b.typ) {
		return builderErr("withholding", "not allowed for invoice type "+b.typ)
	}
	if !hasWH && withholdingRequired(b.typ) {
		return builderErr("withholding", "invoice type "+b.typ+" requires at least one withholding tax")
	}

	if b.currency != codelist.TRY {
		if b.exchange == nil {
			return builderErr("exchangeRate", "required when currency is "+b.currency)
		}
	}
	if b.exchange != nil && b.exchange.SourceCurrencyCode.Value != b.currency {
		return builderErr("exchangeRate.source", "source "+b.exchange.SourceCurrencyCode.Value+" does not match document currency "+b.currency)
	}

	if b.profile == codelist.ProfileIHRACAT {
		if b.buyer == nil {
			return builderErr("buyerCustomer", "required for IHRACAT")
		}
		for _, line := range b.lines {
			if !deliveryHasCustomsID(line.Delivery) {
				return builderErr("line.delivery", "IHRACAT line "+line.ID.Value+" requires a GTİP (DeliveryBuilder.CustomsID)")
			}
		}
	}

	if b.amountWords && b.manual {
		return builderErr("amountInWords", "requires automatic totals")
	}
	return nil
}
