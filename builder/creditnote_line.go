package builder

import (
	"strconv"

	"github.com/furkanmeclis/go-ubltr/schema/ubltr121/cac"
	"github.com/furkanmeclis/go-ubltr/schema/ubltr121/cbc"
)

// CreditNoteLineBuilder constructs a generated [cac.CreditNoteLineType].
type CreditNoteLineBuilder struct {
	err error

	id, name   string
	qty        Quantity
	hasQty     bool
	price      Money
	hasPrice   bool
	taxes      []*TaxBuilder
	allowances []allowanceSpec
}

// NewCreditNoteLineBuilder returns an empty credit-note line builder.
func NewCreditNoteLineBuilder() *CreditNoteLineBuilder {
	return &CreditNoteLineBuilder{
		taxes:      []*TaxBuilder{},
		allowances: []allowanceSpec{},
	}
}

func (b *CreditNoteLineBuilder) fail(field, msg string) {
	if b == nil || b.err != nil {
		return
	}
	b.err = builderErr(field, msg)
}

func (b *CreditNoteLineBuilder) failErr(field string, err error) {
	if b == nil || b.err != nil || err == nil {
		return
	}
	b.err = builderWrap(field, err)
}

// ID sets the line ID. CreditNoteBuilder fills 1, 2, … when empty.
func (b *CreditNoteLineBuilder) ID(id string) *CreditNoteLineBuilder {
	b.id = id
	return b
}

// Name sets Item/Name.
func (b *CreditNoteLineBuilder) Name(name string) *CreditNoteLineBuilder {
	b.name = name
	return b
}

// Quantity sets CreditedQuantity.
func (b *CreditNoteLineBuilder) Quantity(q Quantity) *CreditNoteLineBuilder {
	b.qty = q
	b.hasQty = true
	return b
}

// Price sets the unit PriceAmount.
func (b *CreditNoteLineBuilder) Price(m Money) *CreditNoteLineBuilder {
	b.price = m
	b.hasPrice = true
	return b
}

// Tax appends a tax computed on the line extension.
func (b *CreditNoteLineBuilder) Tax(t *TaxBuilder) *CreditNoteLineBuilder {
	if t == nil {
		b.fail("tax", "nil tax builder")
		return b
	}
	b.taxes = append(b.taxes, t)
	return b
}

// Allowance subtracts a fixed amount from the line extension.
func (b *CreditNoteLineBuilder) Allowance(amount Money, reason string) *CreditNoteLineBuilder {
	b.allowances = append(b.allowances, allowanceSpec{amount: amount, reason: reason, hasAmt: true})
	return b
}

// AllowancePercent subtracts percent of the pre-allowance extension.
func (b *CreditNoteLineBuilder) AllowancePercent(percent string, reason string) *CreditNoteLineBuilder {
	b.allowances = append(b.allowances, allowanceSpec{percent: percent, reason: reason})
	return b
}

// Charge adds a fixed amount to the line extension.
func (b *CreditNoteLineBuilder) Charge(amount Money, reason string) *CreditNoteLineBuilder {
	b.allowances = append(b.allowances, allowanceSpec{charge: true, amount: amount, reason: reason, hasAmt: true})
	return b
}

// ChargePercent adds percent of the pre-allowance extension.
func (b *CreditNoteLineBuilder) ChargePercent(percent string, reason string) *CreditNoteLineBuilder {
	b.allowances = append(b.allowances, allowanceSpec{charge: true, percent: percent, reason: reason})
	return b
}

// Build returns a generated credit-note line. Name, quantity, and price are required.
func (b *CreditNoteLineBuilder) Build() (cac.CreditNoteLineType, error) {
	if b == nil {
		return cac.CreditNoteLineType{}, builderErr("line", "nil builder")
	}
	if b.err != nil {
		return cac.CreditNoteLineType{}, b.err
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
		return cac.CreditNoteLineType{}, b.err
	}

	gross := b.price.TimesQuantity(b.qty)
	charges, err := applyAllowances(gross, b.allowances)
	if err != nil {
		return cac.CreditNoteLineType{}, err
	}
	extAmt := charges.extension.Round()

	applied := make([]appliedTax, 0, len(b.taxes))
	for _, tb := range b.taxes {
		at, err := tb.apply(extAmt)
		if err != nil {
			return cac.CreditNoteLineType{}, err
		}
		applied = append(applied, at)
	}
	var taxes []cac.TaxTotalType
	if len(applied) > 0 {
		tot, _, err := taxTotalOf(applied)
		if err != nil {
			return cac.CreditNoteLineType{}, builderWrap("line.tax", err)
		}
		taxes = []cac.TaxTotalType{tot}
	}

	qty := quantityValue(b.qty)
	amt := amountValue(extAmt, true)
	item := cac.ItemType{Name: cbc.NameType{Value: b.name}}
	price := cac.PriceType{PriceAmount: amountValue(b.price, false)}
	line := cac.CreditNoteLineType{
		ID:                  cbcID(b.id),
		CreditedQuantity:    &qty,
		LineExtensionAmount: &amt,
		AllowanceCharge:     charges.xml,
		TaxTotal:            taxes,
		Item:                &item,
		Price:               &price,
	}
	return line, nil
}

func setCreditNoteLineID(line *cac.CreditNoteLineType, n int) {
	if line.ID.Value == "" {
		line.ID = cbcID(strconv.Itoa(n))
	}
}
