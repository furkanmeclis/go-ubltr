package builder

import (
	"strconv"
	"strings"

	"github.com/furkanmeclis/go-ubltr/schema/ubltr121/cac"
	"github.com/furkanmeclis/go-ubltr/schema/ubltr121/cbc"
	"github.com/furkanmeclis/go-ubltr/schema/ubltr121/receiptadvice"
)

// ReceiptAdviceBuilder constructs a generated ReceiptAdvice.
type ReceiptAdviceBuilder struct {
	err error

	profile, typ, id, uuid   string
	issueDate                string
	despatchID, despatchDate string

	supplier, customer cac.PartyType
	hasSupplier        bool
	hasCustomer        bool
	lines              []cac.ReceiptLineType
}

// NewReceiptAdviceBuilder returns an empty receipt-advice builder.
func NewReceiptAdviceBuilder() *ReceiptAdviceBuilder {
	return &ReceiptAdviceBuilder{
		lines: []cac.ReceiptLineType{},
	}
}

func (b *ReceiptAdviceBuilder) fail(field, msg string) {
	if b == nil || b.err != nil {
		return
	}
	b.err = builderErr(field, msg)
}

func (b *ReceiptAdviceBuilder) failErr(field string, err error) {
	if b == nil || b.err != nil || err == nil {
		return
	}
	b.err = builderWrap(field, err)
}

// Profile sets ProfileID.
func (b *ReceiptAdviceBuilder) Profile(id string) *ReceiptAdviceBuilder {
	b.profile = strings.TrimSpace(id)
	return b
}

// Type sets ReceiptAdviceTypeCode ([codelist.ReceiptSEVK], …).
func (b *ReceiptAdviceBuilder) Type(code string) *ReceiptAdviceBuilder {
	b.typ = strings.TrimSpace(code)
	return b
}

// ID sets the document ID.
func (b *ReceiptAdviceBuilder) ID(id string) *ReceiptAdviceBuilder {
	b.id = strings.TrimSpace(id)
	return b
}

// UUID sets cbc:UUID. Generated on Build when empty.
func (b *ReceiptAdviceBuilder) UUID(id string) *ReceiptAdviceBuilder {
	b.uuid = strings.TrimSpace(id)
	return b
}

// IssueDate sets IssueDate (YYYY-MM-DD).
func (b *ReceiptAdviceBuilder) IssueDate(date string) *ReceiptAdviceBuilder {
	b.issueDate = strings.TrimSpace(date)
	return b
}

// DespatchReference sets the required DespatchDocumentReference.
func (b *ReceiptAdviceBuilder) DespatchReference(id, issueDate string) *ReceiptAdviceBuilder {
	b.despatchID = strings.TrimSpace(id)
	b.despatchDate = strings.TrimSpace(issueDate)
	return b
}

// Supplier sets DespatchSupplierParty.
func (b *ReceiptAdviceBuilder) Supplier(p cac.PartyType) *ReceiptAdviceBuilder {
	b.supplier = p
	b.hasSupplier = true
	return b
}

// SupplierBuilder builds the party and sets the supplier.
func (b *ReceiptAdviceBuilder) SupplierBuilder(pb *PartyBuilder) *ReceiptAdviceBuilder {
	p, err := pb.Build()
	if err != nil {
		b.failErr("supplier", err)
		return b
	}
	return b.Supplier(p)
}

// Customer sets DeliveryCustomerParty.
func (b *ReceiptAdviceBuilder) Customer(p cac.PartyType) *ReceiptAdviceBuilder {
	b.customer = p
	b.hasCustomer = true
	return b
}

// CustomerBuilder builds the party and sets the customer.
func (b *ReceiptAdviceBuilder) CustomerBuilder(pb *PartyBuilder) *ReceiptAdviceBuilder {
	p, err := pb.Build()
	if err != nil {
		b.failErr("customer", err)
		return b
	}
	return b.Customer(p)
}

// Line builds lb and appends a receipt line.
func (b *ReceiptAdviceBuilder) Line(lb *ReceiptLineBuilder) *ReceiptAdviceBuilder {
	line, err := lb.Build()
	if err != nil {
		b.failErr("line", err)
		return b
	}
	if line.ID.Value == "" {
		line.ID = cbcID(strconv.Itoa(len(b.lines) + 1))
	}
	b.lines = append(b.lines, line)
	return b
}

// Build returns a generated *receiptadvice.ReceiptAdvice.
func (b *ReceiptAdviceBuilder) Build() (*receiptadvice.ReceiptAdvice, error) {
	if b == nil {
		return nil, builderErr("receiptadvice", "nil builder")
	}
	if b.err != nil {
		return nil, b.err
	}
	if b.profile == "" {
		b.fail("profile", "required")
	}
	if b.id == "" {
		b.fail("id", "required")
	}
	if b.issueDate == "" {
		b.fail("issueDate", "required")
	}
	if b.despatchID == "" || b.despatchDate == "" {
		b.fail("despatchReference", "required")
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

	uuid := b.uuid
	if uuid == "" {
		id, err := newUUID()
		if err != nil {
			return nil, builderWrap("uuid", err)
		}
		uuid = id
	}

	doc := &receiptadvice.ReceiptAdvice{}
	doc.UBLExtensions = ublExtensions()
	doc.UBLVersionID = cbc.UBLVersionIDType{Value: ublVersionTR}
	doc.CustomizationID = cbc.CustomizationIDType{Value: customizationTR12}
	doc.ProfileID = cbc.ProfileIDType{Value: b.profile}
	doc.ID = cbcID(b.id)
	doc.CopyIndicator = cbc.CopyIndicatorType{Value: "false"}
	doc.UUID = cbc.UUIDType{Value: uuid}
	doc.IssueDate = cbc.IssueDateType{Value: b.issueDate}
	if b.typ != "" {
		doc.ReceiptAdviceTypeCode = ptrOf(cbc.ReceiptAdviceTypeCodeType{Value: b.typ})
	}
	n := cbc.LineCountNumericType{Value: strconv.Itoa(len(b.lines))}
	doc.LineCountNumeric = &n
	doc.DespatchDocumentReference = cac.DocumentReferenceType{
		ID:        cbcID(b.despatchID),
		IssueDate: cbc.IssueDateType{Value: b.despatchDate},
	}
	doc.Signature = []cac.SignatureType{signatureOf(b.customer)}
	doc.DeliveryCustomerParty = cac.CustomerPartyType{Party: b.customer}
	doc.DespatchSupplierParty = cac.SupplierPartyType{Party: b.supplier}
	doc.ReceiptLine = b.lines
	return doc, nil
}

// ReceiptLineBuilder constructs a generated [cac.ReceiptLineType].
type ReceiptLineBuilder struct {
	err error

	id, name string
	qty      Quantity
	hasQty   bool
}

// NewReceiptLineBuilder returns an empty receipt-line builder.
func NewReceiptLineBuilder() *ReceiptLineBuilder {
	return &ReceiptLineBuilder{}
}

func (b *ReceiptLineBuilder) fail(field, msg string) {
	if b == nil || b.err != nil {
		return
	}
	b.err = builderErr(field, msg)
}

// ID sets the line ID.
func (b *ReceiptLineBuilder) ID(id string) *ReceiptLineBuilder {
	b.id = id
	return b
}

// Name sets Item/Name.
func (b *ReceiptLineBuilder) Name(name string) *ReceiptLineBuilder {
	b.name = name
	return b
}

// Quantity sets ReceivedQuantity.
func (b *ReceiptLineBuilder) Quantity(q Quantity) *ReceiptLineBuilder {
	b.qty = q
	b.hasQty = true
	return b
}

// Build returns a generated receipt line.
func (b *ReceiptLineBuilder) Build() (cac.ReceiptLineType, error) {
	if b == nil {
		return cac.ReceiptLineType{}, builderErr("line", "nil builder")
	}
	if b.err != nil {
		return cac.ReceiptLineType{}, b.err
	}
	if b.name == "" {
		b.fail("line.name", "required")
	}
	if !b.hasQty {
		b.fail("line.quantity", "required")
	}
	if b.err != nil {
		return cac.ReceiptLineType{}, b.err
	}
	qty := quantityValue(b.qty)
	return cac.ReceiptLineType{
		ID:               cbcID(b.id),
		ReceivedQuantity: &qty,
		Item:             cac.ItemType{Name: cbc.NameType{Value: b.name}},
	}, nil
}
