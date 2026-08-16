package builder

import (
	"strconv"
	"strings"

	"github.com/furkanmeclis/go-ubltr/schema/ubltr121/cac"
	"github.com/furkanmeclis/go-ubltr/schema/ubltr121/cbc"
	"github.com/furkanmeclis/go-ubltr/schema/ubltr121/despatchadvice"
)

// DespatchAdviceBuilder constructs a generated DespatchAdvice.
type DespatchAdviceBuilder struct {
	err error

	profile, typ, id, uuid string
	issueDate, issueTime   string
	shipmentID             string

	supplier, customer cac.PartyType
	hasSupplier        bool
	hasCustomer        bool
	lines              []cac.DespatchLineType
}

// NewDespatchAdviceBuilder returns an empty despatch-advice builder.
func NewDespatchAdviceBuilder() *DespatchAdviceBuilder {
	return &DespatchAdviceBuilder{
		lines: []cac.DespatchLineType{},
	}
}

func (b *DespatchAdviceBuilder) fail(field, msg string) {
	if b == nil || b.err != nil {
		return
	}
	b.err = builderErr(field, msg)
}

func (b *DespatchAdviceBuilder) failErr(field string, err error) {
	if b == nil || b.err != nil || err == nil {
		return
	}
	b.err = builderWrap(field, err)
}

// Profile sets ProfileID ([codelist.ProfileTemelIrsaliye], …).
func (b *DespatchAdviceBuilder) Profile(id string) *DespatchAdviceBuilder {
	b.profile = strings.TrimSpace(id)
	return b
}

// Type sets DespatchAdviceTypeCode ([codelist.DespatchSEVK], …).
func (b *DespatchAdviceBuilder) Type(code string) *DespatchAdviceBuilder {
	b.typ = strings.TrimSpace(code)
	return b
}

// ID sets the document ID.
func (b *DespatchAdviceBuilder) ID(id string) *DespatchAdviceBuilder {
	b.id = strings.TrimSpace(id)
	return b
}

// UUID sets cbc:UUID. Generated on Build when empty.
func (b *DespatchAdviceBuilder) UUID(id string) *DespatchAdviceBuilder {
	b.uuid = strings.TrimSpace(id)
	return b
}

// IssueDate sets IssueDate (YYYY-MM-DD).
func (b *DespatchAdviceBuilder) IssueDate(date string) *DespatchAdviceBuilder {
	b.issueDate = strings.TrimSpace(date)
	return b
}

// IssueTime sets IssueTime (HH:MM:SS). Required by the GİB DespatchAdvice XSD.
func (b *DespatchAdviceBuilder) IssueTime(t string) *DespatchAdviceBuilder {
	b.issueTime = strings.TrimSpace(t)
	return b
}

// ShipmentID sets Shipment/ID. Required.
func (b *DespatchAdviceBuilder) ShipmentID(id string) *DespatchAdviceBuilder {
	b.shipmentID = strings.TrimSpace(id)
	return b
}

// Supplier sets DespatchSupplierParty.
func (b *DespatchAdviceBuilder) Supplier(p cac.PartyType) *DespatchAdviceBuilder {
	b.supplier = p
	b.hasSupplier = true
	return b
}

// SupplierBuilder builds the party and sets the supplier.
func (b *DespatchAdviceBuilder) SupplierBuilder(pb *PartyBuilder) *DespatchAdviceBuilder {
	p, err := pb.Build()
	if err != nil {
		b.failErr("supplier", err)
		return b
	}
	return b.Supplier(p)
}

// Customer sets DeliveryCustomerParty.
func (b *DespatchAdviceBuilder) Customer(p cac.PartyType) *DespatchAdviceBuilder {
	b.customer = p
	b.hasCustomer = true
	return b
}

// CustomerBuilder builds the party and sets the customer.
func (b *DespatchAdviceBuilder) CustomerBuilder(pb *PartyBuilder) *DespatchAdviceBuilder {
	p, err := pb.Build()
	if err != nil {
		b.failErr("customer", err)
		return b
	}
	return b.Customer(p)
}

// Line builds lb and appends a despatch line.
func (b *DespatchAdviceBuilder) Line(lb *DespatchLineBuilder) *DespatchAdviceBuilder {
	line, err := lb.Build()
	if err != nil {
		b.failErr("line", err)
		return b
	}
	if line.ID.Value == "" {
		line.ID = cbcID(strconv.Itoa(len(b.lines) + 1))
	}
	if line.OrderLineReference.LineID.Value == "" {
		line.OrderLineReference.LineID = cbc.LineIDType{Value: line.ID.Value}
	}
	b.lines = append(b.lines, line)
	return b
}

// Build returns a generated *despatchadvice.DespatchAdvice.
func (b *DespatchAdviceBuilder) Build() (*despatchadvice.DespatchAdvice, error) {
	if b == nil {
		return nil, builderErr("despatchadvice", "nil builder")
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
	if b.issueTime == "" {
		b.fail("issueTime", "required")
	}
	if b.shipmentID == "" {
		b.fail("shipment", "required")
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

	doc := &despatchadvice.DespatchAdvice{}
	doc.UBLExtensions = ublExtensions()
	doc.UBLVersionID = cbc.UBLVersionIDType{Value: ublVersionTR}
	doc.CustomizationID = cbc.CustomizationIDType{Value: customizationTR12}
	doc.ProfileID = cbc.ProfileIDType{Value: b.profile}
	doc.ID = cbcID(b.id)
	doc.CopyIndicator = cbc.CopyIndicatorType{Value: "false"}
	doc.UUID = cbc.UUIDType{Value: uuid}
	doc.IssueDate = cbc.IssueDateType{Value: b.issueDate}
	doc.IssueTime = cbc.IssueTimeType{Value: b.issueTime}
	if b.typ != "" {
		doc.DespatchAdviceTypeCode = ptrOf(cbc.DespatchAdviceTypeCodeType{Value: b.typ})
	}
	n := cbc.LineCountNumericType{Value: strconv.Itoa(len(b.lines))}
	doc.LineCountNumeric = &n
	doc.Signature = []cac.SignatureType{signatureOf(b.supplier)}
	doc.DespatchSupplierParty = cac.SupplierPartyType{Party: b.supplier}
	doc.DeliveryCustomerParty = cac.CustomerPartyType{Party: b.customer}
	doc.Shipment = cac.ShipmentType{ID: cbcID(b.shipmentID)}
	doc.DespatchLine = b.lines
	return doc, nil
}

// DespatchLineBuilder constructs a generated [cac.DespatchLineType].
type DespatchLineBuilder struct {
	err error

	id, name, orderLineID string
	qty                   Quantity
	hasQty                bool
}

// NewDespatchLineBuilder returns an empty despatch-line builder.
func NewDespatchLineBuilder() *DespatchLineBuilder {
	return &DespatchLineBuilder{}
}

func (b *DespatchLineBuilder) fail(field, msg string) {
	if b == nil || b.err != nil {
		return
	}
	b.err = builderErr(field, msg)
}

// ID sets the line ID.
func (b *DespatchLineBuilder) ID(id string) *DespatchLineBuilder {
	b.id = id
	return b
}

// Name sets Item/Name.
func (b *DespatchLineBuilder) Name(name string) *DespatchLineBuilder {
	b.name = name
	return b
}

// Quantity sets DeliveredQuantity.
func (b *DespatchLineBuilder) Quantity(q Quantity) *DespatchLineBuilder {
	b.qty = q
	b.hasQty = true
	return b
}

// OrderLineID sets OrderLineReference/LineID. Defaults to the line ID.
func (b *DespatchLineBuilder) OrderLineID(id string) *DespatchLineBuilder {
	b.orderLineID = id
	return b
}

// Build returns a generated despatch line.
func (b *DespatchLineBuilder) Build() (cac.DespatchLineType, error) {
	if b == nil {
		return cac.DespatchLineType{}, builderErr("line", "nil builder")
	}
	if b.err != nil {
		return cac.DespatchLineType{}, b.err
	}
	if b.name == "" {
		b.fail("line.name", "required")
	}
	if !b.hasQty {
		b.fail("line.quantity", "required")
	}
	if b.err != nil {
		return cac.DespatchLineType{}, b.err
	}
	qty := quantityValue(b.qty)
	orderID := b.orderLineID
	if orderID == "" {
		orderID = b.id
	}
	return cac.DespatchLineType{
		ID:                cbcID(b.id),
		DeliveredQuantity: &qty,
		OrderLineReference: cac.OrderLineReferenceType{
			LineID: cbc.LineIDType{Value: orderID},
		},
		Item: cac.ItemType{Name: cbc.NameType{Value: b.name}},
	}, nil
}
