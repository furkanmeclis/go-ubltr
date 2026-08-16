package builder

import (
	"strconv"
	"strings"

	"github.com/furkanmeclis/go-ubltr/codelist"
	"github.com/furkanmeclis/go-ubltr/schema/ubltr121/cac"
	"github.com/furkanmeclis/go-ubltr/schema/ubltr121/cbc"
	"github.com/furkanmeclis/go-ubltr/schema/ubltr121/creditnote"
)

// CreditNoteBuilder constructs a generated [creditnote.CreditNote].
type CreditNoteBuilder struct {
	err error

	profile, typ, id, uuid string
	issueDate, issueTime   string
	currency               string
	notes                  []string
	periodStart, periodEnd string

	supplier, customer cac.PartyType
	hasSupplier        bool
	hasCustomer        bool

	lines      []cac.CreditNoteLineType
	allowances []allowanceSpec
	means      []cac.PaymentMeansType
	terms      []cac.PaymentTermsType
	deliveries []cac.DeliveryType
	manual     bool
}

// NewCreditNoteBuilder returns a fluent credit-note builder. Defaults match
// [NewInvoiceBuilder]: currency TRY, UBL 2.1 / TR1.2, generated UUID.
func NewCreditNoteBuilder() *CreditNoteBuilder {
	return &CreditNoteBuilder{
		currency:   codelist.TRY,
		notes:      []string{},
		lines:      []cac.CreditNoteLineType{},
		allowances: []allowanceSpec{},
		means:      []cac.PaymentMeansType{},
		terms:      []cac.PaymentTermsType{},
		deliveries: []cac.DeliveryType{},
	}
}

func (b *CreditNoteBuilder) fail(field, msg string) {
	if b == nil || b.err != nil {
		return
	}
	b.err = builderErr(field, msg)
}

func (b *CreditNoteBuilder) failErr(field string, err error) {
	if b == nil || b.err != nil || err == nil {
		return
	}
	b.err = builderWrap(field, err)
}

// Profile sets ProfileID.
func (b *CreditNoteBuilder) Profile(id string) *CreditNoteBuilder {
	b.profile = strings.TrimSpace(id)
	return b
}

// Type sets CreditNoteTypeCode (GİB invoice type codes such as [codelist.InvoiceTypeIade]).
func (b *CreditNoteBuilder) Type(code string) *CreditNoteBuilder {
	b.typ = strings.TrimSpace(code)
	return b
}

// ID sets the document ID.
func (b *CreditNoteBuilder) ID(id string) *CreditNoteBuilder {
	b.id = strings.TrimSpace(id)
	return b
}

// UUID sets cbc:UUID. A v4 UUID is generated on Build when empty.
func (b *CreditNoteBuilder) UUID(id string) *CreditNoteBuilder {
	b.uuid = strings.TrimSpace(id)
	return b
}

// IssueDate sets IssueDate (YYYY-MM-DD).
func (b *CreditNoteBuilder) IssueDate(date string) *CreditNoteBuilder {
	b.issueDate = strings.TrimSpace(date)
	return b
}

// IssueTime sets IssueTime (HH:MM:SS).
func (b *CreditNoteBuilder) IssueTime(t string) *CreditNoteBuilder {
	b.issueTime = strings.TrimSpace(t)
	return b
}

// Currency sets DocumentCurrencyCode. Default [codelist.TRY].
func (b *CreditNoteBuilder) Currency(code string) *CreditNoteBuilder {
	b.currency = strings.ToUpper(strings.TrimSpace(code))
	return b
}

// Note appends cbc:Note.
func (b *CreditNoteBuilder) Note(note string) *CreditNoteBuilder {
	b.notes = append(b.notes, note)
	return b
}

// Period sets InvoicePeriod StartDate and EndDate.
func (b *CreditNoteBuilder) Period(start, end string) *CreditNoteBuilder {
	b.periodStart = start
	b.periodEnd = end
	return b
}

// Supplier sets AccountingSupplierParty.
func (b *CreditNoteBuilder) Supplier(p cac.PartyType) *CreditNoteBuilder {
	b.supplier = p
	b.hasSupplier = true
	return b
}

// SupplierBuilder builds the party and sets the supplier.
func (b *CreditNoteBuilder) SupplierBuilder(pb *PartyBuilder) *CreditNoteBuilder {
	p, err := pb.Build()
	if err != nil {
		b.failErr("supplier", err)
		return b
	}
	return b.Supplier(p)
}

// Customer sets AccountingCustomerParty.
func (b *CreditNoteBuilder) Customer(p cac.PartyType) *CreditNoteBuilder {
	b.customer = p
	b.hasCustomer = true
	return b
}

// CustomerBuilder builds the party and sets the customer.
func (b *CreditNoteBuilder) CustomerBuilder(pb *PartyBuilder) *CreditNoteBuilder {
	p, err := pb.Build()
	if err != nil {
		b.failErr("customer", err)
		return b
	}
	return b.Customer(p)
}

// Line builds lb and appends a credit-note line.
func (b *CreditNoteBuilder) Line(lb *CreditNoteLineBuilder) *CreditNoteBuilder {
	line, err := lb.Build()
	if err != nil {
		b.failErr("line", err)
		return b
	}
	setCreditNoteLineID(&line, len(b.lines)+1)
	b.lines = append(b.lines, line)
	return b
}

// Allowance adds a document-level allowance.
func (b *CreditNoteBuilder) Allowance(amount Money, reason string) *CreditNoteBuilder {
	b.allowances = append(b.allowances, allowanceSpec{amount: amount, reason: reason, hasAmt: true})
	return b
}

// Charge adds a document-level charge.
func (b *CreditNoteBuilder) Charge(amount Money, reason string) *CreditNoteBuilder {
	b.allowances = append(b.allowances, allowanceSpec{charge: true, amount: amount, reason: reason, hasAmt: true})
	return b
}

// Payment adds PaymentMeans and/or PaymentTerms from pb.
func (b *CreditNoteBuilder) Payment(pb *PaymentBuilder) *CreditNoteBuilder {
	if pb == nil {
		b.fail("payment", "nil builder")
		return b
	}
	if pb.code != "" {
		m, err := pb.BuildMeans()
		if err != nil {
			b.failErr("payment", err)
			return b
		}
		b.means = append(b.means, m)
	}
	if pb.due != "" || pb.note != "" {
		t, err := pb.BuildTerms()
		if err != nil {
			b.failErr("payment.terms", err)
			return b
		}
		b.terms = append(b.terms, t)
	}
	return b
}

// Delivery appends a delivery built from db.
func (b *CreditNoteBuilder) Delivery(db *DeliveryBuilder) *CreditNoteBuilder {
	d, err := db.Build()
	if err != nil {
		b.failErr("delivery", err)
		return b
	}
	b.deliveries = append(b.deliveries, d)
	return b
}

// ManualTotals skips automatic LegalMonetaryTotal and TaxTotal.
func (b *CreditNoteBuilder) ManualTotals() *CreditNoteBuilder {
	b.manual = true
	return b
}

// Build returns a generated *creditnote.CreditNote. It does not run XSD or Schematron.
func (b *CreditNoteBuilder) Build() (*creditnote.CreditNote, error) {
	if b == nil {
		return nil, builderErr("creditnote", "nil builder")
	}
	if b.err != nil {
		return nil, b.err
	}
	if b.profile == "" {
		b.fail("profile", "required")
	}
	if b.typ == "" {
		b.fail("type", "required")
	}
	if b.id == "" {
		b.fail("id", "required")
	}
	if b.issueDate == "" {
		b.fail("issueDate", "required")
	}
	if b.currency == "" {
		b.fail("currency", "required")
	}
	if !b.hasSupplier {
		b.fail("supplier", "required")
	}
	if !b.hasCustomer {
		b.fail("customer", "required")
	}
	if len(b.lines) == 0 {
		b.fail("line", "at least one line is required")
	}
	if b.err != nil {
		return nil, b.err
	}

	if err := b.checkLineCurrency(); err != nil {
		return nil, err
	}

	uuid := b.uuid
	if uuid == "" {
		id, err := newUUID()
		if err != nil {
			return nil, builderWrap("uuid", err)
		}
		uuid = id
	}

	notes := make([]cbc.NoteType, 0, len(b.notes))
	for _, n := range b.notes {
		notes = append(notes, cbc.NoteType{Value: n})
	}

	cn := &creditnote.CreditNote{}
	cn.UBLExtensions = ptrExtensions()
	cn.UBLVersionID = ptrOf(cbc.UBLVersionIDType{Value: ublVersionTR})
	cn.CustomizationID = ptrOf(cbc.CustomizationIDType{Value: customizationTR12})
	cn.ProfileID = ptrOf(cbc.ProfileIDType{Value: b.profile})
	cn.ID = cbcID(b.id)
	cn.CopyIndicator = ptrOf(cbc.CopyIndicatorType{Value: "false"})
	cn.UUID = ptrOf(cbc.UUIDType{Value: uuid})
	cn.IssueDate = cbc.IssueDateType{Value: b.issueDate}
	if b.issueTime != "" {
		cn.IssueTime = ptrOf(cbc.IssueTimeType{Value: b.issueTime})
	}
	cn.CreditNoteTypeCode = ptrOf(cbc.CreditNoteTypeCodeType{Value: b.typ})
	cn.Note = notes
	cn.DocumentCurrencyCode = ptrOf(cbc.DocumentCurrencyCodeType{Value: b.currency})
	cn.LineCountNumeric = ptrOf(cbc.LineCountNumericType{Value: strconv.Itoa(len(b.lines))})
	if b.periodStart != "" || b.periodEnd != "" {
		cn.InvoicePeriod = []cac.PeriodType{{
			StartDate: optStart(b.periodStart),
			EndDate:   optEnd(b.periodEnd),
		}}
	}
	cn.Signature = []cac.SignatureType{signatureOf(b.supplier)}
	cn.AccountingSupplierParty = cac.SupplierPartyType{Party: b.supplier}
	cn.AccountingCustomerParty = cac.CustomerPartyType{Party: b.customer}
	cn.Delivery = b.deliveries
	cn.PaymentMeans = b.means
	cn.PaymentTerms = b.terms
	cn.CreditNoteLine = b.lines

	if !b.manual {
		if err := b.applyTotals(cn); err != nil {
			return nil, err
		}
	}

	return cn, nil
}

func (b *CreditNoteBuilder) checkLineCurrency() error {
	for i, line := range b.lines {
		if line.LineExtensionAmount != nil && line.LineExtensionAmount.CurrencyID != b.currency {
			return builderErr("line.currency", "line "+strconv.Itoa(i+1)+" currency "+line.LineExtensionAmount.CurrencyID+" does not match "+b.currency)
		}
		if line.Price != nil && line.Price.PriceAmount.CurrencyID != "" && line.Price.PriceAmount.CurrencyID != b.currency {
			return builderErr("line.price", "line "+strconv.Itoa(i+1)+" price currency mismatch")
		}
	}
	return nil
}

func (b *CreditNoteBuilder) applyTotals(cn *creditnote.CreditNote) error {
	lineExt := ZeroMoney(b.currency)
	for _, line := range b.lines {
		if line.LineExtensionAmount == nil {
			continue
		}
		amt, err := parseAmount(*line.LineExtensionAmount)
		if err != nil {
			return builderWrap("line.extension", err)
		}
		lineExt, err = lineExt.Add(amt)
		if err != nil {
			return builderWrap("totals", err)
		}
	}
	lineExt = lineExt.Round()

	doc, err := applyAllowances(lineExt, b.allowances)
	if err != nil {
		return err
	}
	cn.AllowanceCharge = doc.xml

	taxExclusive := doc.extension.Round()
	taxXML, taxSum, err := mergeCreditNoteTax(b.lines)
	if err != nil {
		return builderWrap("tax", err)
	}
	if taxSum.currency == "" {
		taxSum = ZeroMoney(b.currency).Round()
		taxXML = cac.TaxTotalType{
			TaxAmount:   emptyAmount(b.currency),
			TaxSubtotal: []cac.TaxSubtotalType{},
		}
	}
	cn.TaxTotal = []cac.TaxTotalType{taxXML}

	taxInclusive, err := taxExclusive.Add(taxSum)
	if err != nil {
		return builderWrap("totals", err)
	}
	taxInclusive = taxInclusive.Round()

	total := cac.MonetaryTotalType{
		LineExtensionAmount: amountValue(lineExt, true),
		TaxExclusiveAmount:  amountValue(taxExclusive, true),
		TaxInclusiveAmount:  amountValue(taxInclusive, true),
		PayableAmount:       amountValue(taxInclusive, true),
	}
	if !doc.allowances.IsZero() {
		a := amountValue(doc.allowances.Round(), true)
		total.AllowanceTotalAmount = &a
	}
	if !doc.charges.IsZero() {
		a := amountValue(doc.charges.Round(), true)
		total.ChargeTotalAmount = &a
	}
	cn.LegalMonetaryTotal = total
	return nil
}

func signatureOf(p cac.PartyType) cac.SignatureType {
	return cac.SignatureType{
		ID:             signatureID(p),
		SignatoryParty: p,
		DigitalSignatureAttachment: cac.AttachmentType{
			ExternalReference: &cac.ExternalReferenceType{
				URI: cbc.URIType{Value: "#Signature"},
			},
		},
	}
}
