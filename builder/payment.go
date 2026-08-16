package builder

import (
	"github.com/furkanmeclis/go-ubltr/schema/ubltr121/cac"
	"github.com/furkanmeclis/go-ubltr/schema/ubltr121/cbc"
)

// PaymentBuilder constructs PaymentMeans and/or PaymentTerms.
type PaymentBuilder struct {
	err error

	code, due, iban, note string
}

// NewPaymentBuilder returns an empty payment builder.
func NewPaymentBuilder() *PaymentBuilder {
	return &PaymentBuilder{}
}

func (b *PaymentBuilder) fail(field, msg string) {
	if b == nil || b.err != nil {
		return
	}
	b.err = builderErr(field, msg)
}

// Code sets PaymentMeansCode (for example [codelist.PaymentMeansCreditTransfer]).
func (b *PaymentBuilder) Code(code string) *PaymentBuilder {
	b.code = code
	return b
}

// DueDate sets PaymentDueDate on both means and terms when present.
func (b *PaymentBuilder) DueDate(date string) *PaymentBuilder {
	b.due = date
	return b
}

// IBAN sets PayeeFinancialAccount/ID.
func (b *PaymentBuilder) IBAN(iban string) *PaymentBuilder {
	b.iban = iban
	return b
}

// Note sets PaymentTerms/Note (and InstructionNote on means when no terms-only use).
func (b *PaymentBuilder) Note(note string) *PaymentBuilder {
	b.note = note
	return b
}

// BuildMeans returns a generated PaymentMeansType. Code is required.
func (b *PaymentBuilder) BuildMeans() (cac.PaymentMeansType, error) {
	if b == nil {
		return cac.PaymentMeansType{}, builderErr("payment", "nil builder")
	}
	if b.err != nil {
		return cac.PaymentMeansType{}, b.err
	}
	if b.code == "" {
		b.fail("payment.code", "required")
		return cac.PaymentMeansType{}, b.err
	}
	m := cac.PaymentMeansType{
		PaymentMeansCode: cbc.PaymentMeansCodeType{Value: b.code},
		PaymentDueDate:   optDueDate(b.due),
		InstructionNote:  optInstruction(b.note),
	}
	if b.iban != "" {
		m.PayeeFinancialAccount = &cac.FinancialAccountType{ID: cbcID(b.iban)}
	}
	return m, nil
}

// BuildTerms returns a generated PaymentTermsType. Due date or note is required.
func (b *PaymentBuilder) BuildTerms() (cac.PaymentTermsType, error) {
	if b == nil {
		return cac.PaymentTermsType{}, builderErr("payment", "nil builder")
	}
	if b.err != nil {
		return cac.PaymentTermsType{}, b.err
	}
	if b.due == "" && b.note == "" {
		return cac.PaymentTermsType{}, builderErr("payment.terms", "due date or note is required")
	}
	return cac.PaymentTermsType{
		Note:           optNote(b.note),
		PaymentDueDate: optDueDate(b.due),
	}, nil
}
