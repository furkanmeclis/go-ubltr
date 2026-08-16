package builder

import (
	"strconv"
	"strings"

	"github.com/furkanmeclis/go-ubltr/codelist"
	"github.com/furkanmeclis/go-ubltr/schema/ubltr121/cac"
	"github.com/furkanmeclis/go-ubltr/schema/ubltr121/cbc"
	"github.com/furkanmeclis/go-ubltr/schema/ubltr121/ext"
	"github.com/furkanmeclis/go-ubltr/schema/ubltr121/invoice"
)

const (
	ublVersionTR      = "2.1"
	customizationTR12 = "TR1.2"
	// extensionContentPlaceholder is one ##other element. UBL requires
	// ExtensionContent to contain a child outside the ext namespace.
	extensionContentPlaceholder = `<UBLTR xmlns="urn:ubltr:extension"/>`
)

// InvoiceBuilder constructs a generated [invoice.Invoice].
type InvoiceBuilder struct {
	err error

	profile, typ, id, uuid string
	issueDate, issueTime   string
	currency               string
	notes                  []string
	periodStart, periodEnd string

	supplier, customer cac.PartyType
	hasSupplier        bool
	hasCustomer        bool

	lines      []cac.InvoiceLineType
	allowances []allowanceSpec
	means      []cac.PaymentMeansType
	terms      *cac.PaymentTermsType
	deliveries []cac.DeliveryType
	manual     bool
}

// NewInvoiceBuilder returns a fluent invoice builder. Defaults: currency TRY,
// UBL 2.1 / TR1.2, CopyIndicator false, generated UUID, signature from supplier.
func NewInvoiceBuilder() *InvoiceBuilder {
	return &InvoiceBuilder{
		currency:   codelist.TRY,
		notes:      []string{},
		lines:      []cac.InvoiceLineType{},
		allowances: []allowanceSpec{},
		means:      []cac.PaymentMeansType{},
		deliveries: []cac.DeliveryType{},
	}
}

func (b *InvoiceBuilder) fail(field, msg string) {
	if b == nil || b.err != nil {
		return
	}
	b.err = builderErr(field, msg)
}

func (b *InvoiceBuilder) failErr(field string, err error) {
	if b == nil || b.err != nil || err == nil {
		return
	}
	b.err = builderWrap(field, err)
}

// Profile sets ProfileID ([codelist.ProfileTemelFatura], [codelist.ProfileEArsivFatura], …).
func (b *InvoiceBuilder) Profile(id string) *InvoiceBuilder {
	b.profile = strings.TrimSpace(id)
	return b
}

// Type sets InvoiceTypeCode ([codelist.InvoiceTypeSatis], …).
func (b *InvoiceBuilder) Type(code string) *InvoiceBuilder {
	b.typ = strings.TrimSpace(code)
	return b
}

// ID sets the invoice ID (GİB 16-character number).
func (b *InvoiceBuilder) ID(id string) *InvoiceBuilder {
	b.id = strings.TrimSpace(id)
	return b
}

// UUID sets cbc:UUID. A v4 UUID is generated on Build when empty.
func (b *InvoiceBuilder) UUID(id string) *InvoiceBuilder {
	b.uuid = strings.TrimSpace(id)
	return b
}

// IssueDate sets IssueDate (YYYY-MM-DD).
func (b *InvoiceBuilder) IssueDate(date string) *InvoiceBuilder {
	b.issueDate = strings.TrimSpace(date)
	return b
}

// IssueTime sets IssueTime (HH:MM:SS).
func (b *InvoiceBuilder) IssueTime(t string) *InvoiceBuilder {
	b.issueTime = strings.TrimSpace(t)
	return b
}

// Currency sets DocumentCurrencyCode. Default [codelist.TRY].
func (b *InvoiceBuilder) Currency(code string) *InvoiceBuilder {
	b.currency = strings.ToUpper(strings.TrimSpace(code))
	return b
}

// Note appends cbc:Note.
func (b *InvoiceBuilder) Note(note string) *InvoiceBuilder {
	b.notes = append(b.notes, note)
	return b
}

// Period sets InvoicePeriod StartDate and EndDate.
func (b *InvoiceBuilder) Period(start, end string) *InvoiceBuilder {
	b.periodStart = start
	b.periodEnd = end
	return b
}

// Supplier sets AccountingSupplierParty.
func (b *InvoiceBuilder) Supplier(p cac.PartyType) *InvoiceBuilder {
	b.supplier = p
	b.hasSupplier = true
	return b
}

// SupplierBuilder builds the party and sets the supplier.
func (b *InvoiceBuilder) SupplierBuilder(pb *PartyBuilder) *InvoiceBuilder {
	p, err := pb.Build()
	if err != nil {
		b.failErr("supplier", err)
		return b
	}
	return b.Supplier(p)
}

// Customer sets AccountingCustomerParty.
func (b *InvoiceBuilder) Customer(p cac.PartyType) *InvoiceBuilder {
	b.customer = p
	b.hasCustomer = true
	return b
}

// CustomerBuilder builds the party and sets the customer.
func (b *InvoiceBuilder) CustomerBuilder(pb *PartyBuilder) *InvoiceBuilder {
	p, err := pb.Build()
	if err != nil {
		b.failErr("customer", err)
		return b
	}
	return b.Customer(p)
}

// Line builds lb and appends an invoice line.
func (b *InvoiceBuilder) Line(lb *InvoiceLineBuilder) *InvoiceBuilder {
	line, err := lb.Build()
	if err != nil {
		b.failErr("line", err)
		return b
	}
	setLineID(&line, len(b.lines)+1)
	b.lines = append(b.lines, line)
	return b
}

// Allowance adds a document-level allowance (ChargeIndicator false).
func (b *InvoiceBuilder) Allowance(amount Money, reason string) *InvoiceBuilder {
	b.allowances = append(b.allowances, allowanceSpec{amount: amount, reason: reason, hasAmt: true})
	return b
}

// Charge adds a document-level charge (ChargeIndicator true).
func (b *InvoiceBuilder) Charge(amount Money, reason string) *InvoiceBuilder {
	b.allowances = append(b.allowances, allowanceSpec{charge: true, amount: amount, reason: reason, hasAmt: true})
	return b
}

// Payment adds PaymentMeans and PaymentTerms from pb.
func (b *InvoiceBuilder) Payment(pb *PaymentBuilder) *InvoiceBuilder {
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
		b.terms = &t
	}
	return b
}

// Delivery appends a delivery built from db.
func (b *InvoiceBuilder) Delivery(db *DeliveryBuilder) *InvoiceBuilder {
	d, err := db.Build()
	if err != nil {
		b.failErr("delivery", err)
		return b
	}
	b.deliveries = append(b.deliveries, d)
	return b
}

// ManualTotals skips automatic LegalMonetaryTotal and TaxTotal. The caller
// must fill those fields on the returned invoice.
func (b *InvoiceBuilder) ManualTotals() *InvoiceBuilder {
	b.manual = true
	return b
}

// Build returns a generated *invoice.Invoice. It does not run XSD or Schematron.
func (b *InvoiceBuilder) Build() (*invoice.Invoice, error) {
	if b == nil {
		return nil, builderErr("invoice", "nil builder")
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

	inv := &invoice.Invoice{}
	inv.UBLExtensions = ext.UBLExtensionsType{
		UBLExtension: []ext.UBLExtensionType{{
			ExtensionContent: ext.ExtensionContentType{
				InnerXML: extensionContentPlaceholder,
			},
		}},
	}
	inv.UBLVersionID = cbc.UBLVersionIDType{Value: ublVersionTR}
	inv.CustomizationID = cbc.CustomizationIDType{Value: customizationTR12}
	inv.ProfileID = cbc.ProfileIDType{Value: b.profile}
	inv.ID = cbcID(b.id)
	inv.CopyIndicator = cbc.CopyIndicatorType{Value: "false"}
	inv.UUID = cbc.UUIDType{Value: uuid}
	inv.IssueDate = cbc.IssueDateType{Value: b.issueDate}
	if b.issueTime != "" {
		t := cbc.IssueTimeType{Value: b.issueTime}
		inv.IssueTime = &t
	}
	inv.InvoiceTypeCode = cbc.InvoiceTypeCodeType{Value: b.typ}
	inv.Note = notes
	inv.DocumentCurrencyCode = cbc.DocumentCurrencyCodeType{Value: b.currency}
	inv.LineCountNumeric = cbc.LineCountNumericType{Value: strconv.Itoa(len(b.lines))}
	if b.periodStart != "" || b.periodEnd != "" {
		inv.InvoicePeriod = &cac.PeriodType{
			StartDate: optStart(b.periodStart),
			EndDate:   optEnd(b.periodEnd),
		}
	}
	inv.Signature = []cac.SignatureType{b.signature()}
	inv.AccountingSupplierParty = cac.SupplierPartyType{Party: b.supplier}
	inv.AccountingCustomerParty = cac.CustomerPartyType{Party: b.customer}
	inv.Delivery = b.deliveries
	inv.PaymentMeans = b.means
	inv.PaymentTerms = b.terms
	inv.InvoiceLine = b.lines

	if !b.manual {
		if err := b.applyTotals(inv); err != nil {
			return nil, err
		}
	}

	return inv, nil
}

func (b *InvoiceBuilder) checkLineCurrency() error {
	for i, line := range b.lines {
		if line.LineExtensionAmount.CurrencyID != b.currency {
			return builderErr("line.currency", "line "+strconv.Itoa(i+1)+" currency "+line.LineExtensionAmount.CurrencyID+" does not match "+b.currency)
		}
		if line.Price.PriceAmount.CurrencyID != "" && line.Price.PriceAmount.CurrencyID != b.currency {
			return builderErr("line.price", "line "+strconv.Itoa(i+1)+" price currency mismatch")
		}
	}
	return nil
}

func (b *InvoiceBuilder) applyTotals(inv *invoice.Invoice) error {
	lineExt := ZeroMoney(b.currency)
	for _, line := range b.lines {
		amt, err := parseAmount(line.LineExtensionAmount)
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
	inv.AllowanceCharge = doc.xml

	taxExclusive := doc.extension.Round()
	taxXML, taxSum, err := mergeDocumentTax(b.lines)
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
	inv.TaxTotal = []cac.TaxTotalType{taxXML}

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
	inv.LegalMonetaryTotal = total
	return nil
}

func (b *InvoiceBuilder) signature() cac.SignatureType {
	return cac.SignatureType{
		ID:             signatureID(b.supplier),
		SignatoryParty: b.supplier,
		DigitalSignatureAttachment: cac.AttachmentType{
			ExternalReference: &cac.ExternalReferenceType{
				URI: cbc.URIType{Value: "#Signature"},
			},
		},
	}
}

func signatureID(p cac.PartyType) cbc.IDType {
	for _, id := range p.PartyIdentification {
		if id.ID.SchemeID == codelist.SchemeVKN || id.ID.SchemeID == codelist.SchemeTCKN {
			return cbcIDScheme("VKN_TCKN", id.ID.Value)
		}
	}
	if len(p.PartyIdentification) > 0 {
		return cbcIDScheme("VKN_TCKN", p.PartyIdentification[0].ID.Value)
	}
	return cbcIDScheme("VKN_TCKN", "")
}

func optStart(s string) *cbc.StartDateType {
	if s == "" {
		return nil
	}
	v := cbc.StartDateType{Value: s}
	return &v
}

func optEnd(s string) *cbc.EndDateType {
	if s == "" {
		return nil
	}
	v := cbc.EndDateType{Value: s}
	return &v
}
