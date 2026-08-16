package builder

import (
	"strconv"

	"github.com/furkanmeclis/go-ubltr/schema/ubltr121/cac"
	"github.com/furkanmeclis/go-ubltr/schema/ubltr121/cbc"
)

type allowanceSpec struct {
	charge  bool
	amount  Money
	percent string
	reason  string
	hasAmt  bool
}

// InvoiceLineBuilder constructs a generated [cac.InvoiceLineType].
type InvoiceLineBuilder struct {
	err error

	id, name   string
	qty        Quantity
	hasQty     bool
	price      Money
	hasPrice   bool
	taxes      []*TaxBuilder
	allowances []allowanceSpec
}

// NewInvoiceLineBuilder returns an empty line builder.
func NewInvoiceLineBuilder() *InvoiceLineBuilder {
	return &InvoiceLineBuilder{
		taxes:      []*TaxBuilder{},
		allowances: []allowanceSpec{},
	}
}

func (b *InvoiceLineBuilder) fail(field, msg string) {
	if b == nil || b.err != nil {
		return
	}
	b.err = builderErr(field, msg)
}

func (b *InvoiceLineBuilder) failErr(field string, err error) {
	if b == nil || b.err != nil || err == nil {
		return
	}
	b.err = builderWrap(field, err)
}

// ID sets the line ID. InvoiceBuilder fills 1, 2, … when empty.
func (b *InvoiceLineBuilder) ID(id string) *InvoiceLineBuilder {
	b.id = id
	return b
}

// Name sets Item/Name.
func (b *InvoiceLineBuilder) Name(name string) *InvoiceLineBuilder {
	b.name = name
	return b
}

// Quantity sets InvoicedQuantity.
func (b *InvoiceLineBuilder) Quantity(q Quantity) *InvoiceLineBuilder {
	b.qty = q
	b.hasQty = true
	return b
}

// Price sets the unit PriceAmount. Extra scale is kept until line extension is rounded.
func (b *InvoiceLineBuilder) Price(m Money) *InvoiceLineBuilder {
	b.price = m
	b.hasPrice = true
	return b
}

// Tax appends a tax (usually KDV) computed on the line extension.
func (b *InvoiceLineBuilder) Tax(t *TaxBuilder) *InvoiceLineBuilder {
	if t == nil {
		b.fail("tax", "nil tax builder")
		return b
	}
	b.taxes = append(b.taxes, t)
	return b
}

// Allowance subtracts a fixed amount from the line extension.
func (b *InvoiceLineBuilder) Allowance(amount Money, reason string) *InvoiceLineBuilder {
	b.allowances = append(b.allowances, allowanceSpec{amount: amount, reason: reason, hasAmt: true})
	return b
}

// AllowancePercent subtracts percent of the pre-allowance extension.
func (b *InvoiceLineBuilder) AllowancePercent(percent string, reason string) *InvoiceLineBuilder {
	b.allowances = append(b.allowances, allowanceSpec{percent: percent, reason: reason})
	return b
}

// Charge adds a fixed amount to the line extension.
func (b *InvoiceLineBuilder) Charge(amount Money, reason string) *InvoiceLineBuilder {
	b.allowances = append(b.allowances, allowanceSpec{charge: true, amount: amount, reason: reason, hasAmt: true})
	return b
}

// ChargePercent adds percent of the pre-allowance extension.
func (b *InvoiceLineBuilder) ChargePercent(percent string, reason string) *InvoiceLineBuilder {
	b.allowances = append(b.allowances, allowanceSpec{charge: true, percent: percent, reason: reason})
	return b
}

// Build returns a generated invoice line. Name, quantity, and price are required.
func (b *InvoiceLineBuilder) Build() (cac.InvoiceLineType, error) {
	if b == nil {
		return cac.InvoiceLineType{}, builderErr("line", "nil builder")
	}
	if b.err != nil {
		return cac.InvoiceLineType{}, b.err
	}
	if b.name == "" {
		b.fail("line.name", "required")
	}
	if !b.hasQty {
		b.fail("line.quantity", "required")
	}
	if !b.hasPrice {
		b.fail("line.price", "required")
	}
	if b.err != nil {
		return cac.InvoiceLineType{}, b.err
	}

	gross := b.price.TimesQuantity(b.qty)
	charges, err := applyAllowances(gross, b.allowances)
	if err != nil {
		return cac.InvoiceLineType{}, err
	}
	ext := charges.extension.Round()

	applied := make([]appliedTax, 0, len(b.taxes))
	for _, tb := range b.taxes {
		at, err := tb.apply(ext)
		if err != nil {
			return cac.InvoiceLineType{}, err
		}
		applied = append(applied, at)
	}
	var tax *cac.TaxTotalType
	if len(applied) > 0 {
		tot, _, err := taxTotalOf(applied)
		if err != nil {
			return cac.InvoiceLineType{}, builderWrap("line.tax", err)
		}
		tax = &tot
	}

	id := b.id
	line := cac.InvoiceLineType{
		ID:                  cbcID(id),
		InvoicedQuantity:    quantityValue(b.qty),
		LineExtensionAmount: amountValue(ext, true),
		AllowanceCharge:     charges.xml,
		TaxTotal:            tax,
		Item:                cac.ItemType{Name: cbc.NameType{Value: b.name}},
		Price:               cac.PriceType{PriceAmount: amountValue(b.price, false)},
	}
	return line, nil
}

func setLineID(line *cac.InvoiceLineType, n int) {
	if line.ID.Value == "" {
		line.ID = cbcID(strconv.Itoa(n))
	}
}
