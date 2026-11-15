package builder

import (
	"strings"

	"github.com/shopspring/decimal"

	"github.com/furkanmeclis/go-ubltr/codelist"
	"github.com/furkanmeclis/go-ubltr/schema/ubltr121/cac"
	"github.com/furkanmeclis/go-ubltr/schema/ubltr121/cbc"
	"github.com/furkanmeclis/go-ubltr/schema/ubltr121/udt"
)

// TaxBuilder describes one tax (typically KDV) applied to a taxable amount.
type TaxBuilder struct {
	err error

	code, name, percent string
	exemptionCode       string
	exemptionReason     string

	withholdingCode    string
	withholdingPercent string
	withholdingName    string
}

// NewTaxBuilder returns an empty tax builder.
func NewTaxBuilder() *TaxBuilder {
	return &TaxBuilder{}
}

func (b *TaxBuilder) fail(field, msg string) {
	if b == nil || b.err != nil {
		return
	}
	b.err = builderErr(field, msg)
}

// Code sets TaxTypeCode (for example [codelist.TaxKDV] / 0015).
func (b *TaxBuilder) Code(code string) *TaxBuilder {
	b.code = code
	return b
}

// Name sets TaxScheme/Name (defaults to "KDV" when code is [codelist.TaxKDV]).
func (b *TaxBuilder) Name(name string) *TaxBuilder {
	b.name = name
	return b
}

// Percent sets the tax rate as a decimal string ("20", "18.0").
func (b *TaxBuilder) Percent(p string) *TaxBuilder {
	b.percent = p
	return b
}

// KDV is Code([codelist.TaxKDV]) plus Percent.
func (b *TaxBuilder) KDV(percent string) *TaxBuilder {
	return b.Code(codelist.TaxKDV).Percent(percent)
}

// Exemption sets TaxExemptionReasonCode and TaxExemptionReason. The code must
// be a GİB TaxExemptionReasonCodeType value ([codelist.Exemption301], …).
func (b *TaxBuilder) Exemption(code, reason string) *TaxBuilder {
	code = strings.TrimSpace(code)
	if code != "" && !codelist.ValidExemption(code) && !codelist.ValidExemptionYatirim(code) {
		b.fail("tax.exemption", "unknown exemption code "+code)
		return b
	}
	b.exemptionCode = code
	b.exemptionReason = reason
	return b
}

// Withholding marks part of this tax as withheld by the buyer (KDV tevkifatı).
// code is a GİB WithholdingTaxType value ([codelist.Withholding624], …) and
// percent is the withheld share of the tax ("90" for 9/10). The pair must be
// listed in WithholdingTaxTypeWithPercent. The withheld amount is
// tax amount × percent / 100; it is written to cac:WithholdingTaxTotal on the
// line and summed on the invoice, where it is subtracted from PayableAmount.
func (b *TaxBuilder) Withholding(code, percent string) *TaxBuilder {
	code = strings.TrimSpace(code)
	if !codelist.ValidWithholdingTax(code) {
		b.fail("tax.withholding", "unknown withholding code "+code)
		return b
	}
	p, err := decimal.NewFromString(strings.TrimSpace(percent))
	if err != nil {
		b.fail("tax.withholding.percent", "invalid percent "+percent)
		return b
	}
	canonical := p.String()
	if !codelist.ValidWithholdingPercent(code + canonical) {
		b.fail("tax.withholding.percent", "percent "+canonical+" is not allowed for withholding code "+code)
		return b
	}
	b.withholdingCode = code
	b.withholdingPercent = canonical
	return b
}

// WithholdingName sets the withholding TaxScheme/Name. Default "KDV TEVKİFAT".
func (b *TaxBuilder) WithholdingName(name string) *TaxBuilder {
	b.withholdingName = name
	return b
}

func (b *TaxBuilder) hasWithholding() bool {
	return b != nil && b.withholdingCode != ""
}

type appliedTax struct {
	subtotal    cac.TaxSubtotalType
	tax         Money
	taxable     Money
	withholding *appliedTax
}

func (b *TaxBuilder) apply(taxable Money) (appliedTax, error) {
	if b == nil {
		return appliedTax{}, builderErr("tax", "nil builder")
	}
	if b.err != nil {
		return appliedTax{}, b.err
	}
	if b.code == "" {
		b.fail("tax.code", "required")
	}
	if b.percent == "" {
		b.fail("tax.percent", "required")
	}
	if b.err != nil {
		return appliedTax{}, b.err
	}

	tax, err := taxable.PercentOf(b.percent)
	if err != nil {
		return appliedTax{}, builderWrap("tax.percent", err)
	}
	tax = tax.Round()
	taxable = taxable.Round()
	name := b.name
	if name == "" {
		name = defaultTaxName(b.code)
	}

	sub := cac.TaxSubtotalType{
		TaxableAmount: ptrAmount(taxable),
		TaxAmount:     amountValue(tax, true),
		Percent:       optPercent(b.percent),
		TaxCategory: cac.TaxCategoryType{
			TaxExemptionReasonCode: optExemptionCode(b.exemptionCode),
			TaxExemptionReason:     optExemptionReason(b.exemptionReason),
			TaxScheme: cac.TaxSchemeType{
				Name:        optText(name),
				TaxTypeCode: taxTypeCode(b.code),
			},
		},
	}
	out := appliedTax{subtotal: sub, tax: tax, taxable: taxable}
	if b.withholdingCode != "" {
		wh, err := tax.PercentOf(b.withholdingPercent)
		if err != nil {
			return appliedTax{}, builderWrap("tax.withholding.percent", err)
		}
		wh = wh.Round()
		whName := b.withholdingName
		if whName == "" {
			whName = defaultWithholdingName
		}
		out.withholding = &appliedTax{
			tax:     wh,
			taxable: tax,
			subtotal: cac.TaxSubtotalType{
				TaxableAmount: ptrAmount(tax),
				TaxAmount:     amountValue(wh, true),
				Percent:       optPercent(b.withholdingPercent),
				TaxCategory: cac.TaxCategoryType{
					TaxScheme: cac.TaxSchemeType{
						Name:        optText(whName),
						TaxTypeCode: taxTypeCode(b.withholdingCode),
					},
				},
			},
		}
	}
	return out, nil
}

// defaultWithholdingName is the TaxScheme/Name written for withholding subtotals.
const defaultWithholdingName = "KDV TEVKİFAT"

func withholdingTotalOf(taxes []appliedTax) (*cac.TaxTotalType, error) {
	whs := make([]appliedTax, 0, len(taxes))
	for _, t := range taxes {
		if t.withholding != nil {
			whs = append(whs, *t.withholding)
		}
	}
	if len(whs) == 0 {
		return nil, nil
	}
	tot, _, err := taxTotalOf(whs)
	if err != nil {
		return nil, err
	}
	return &tot, nil
}

func taxTypeCode(code string) *cbc.TaxTypeCodeType {
	v := cbc.TaxTypeCodeType{Value: code}
	return &v
}

func ptrAmount(m Money) *cbc.TaxableAmountType {
	a := amountValue(m, true)
	return &a
}

func taxTotalOf(taxes []appliedTax) (cac.TaxTotalType, Money, error) {
	if len(taxes) == 0 {
		return cac.TaxTotalType{}, Money{}, nil
	}
	cur := taxes[0].tax.currency
	sum := ZeroMoney(cur)
	subs := make([]cac.TaxSubtotalType, 0, len(taxes))
	var err error
	for _, t := range taxes {
		sum, err = sum.Add(t.tax)
		if err != nil {
			return cac.TaxTotalType{}, Money{}, err
		}
		subs = append(subs, t.subtotal)
	}
	sum = sum.Round()
	return cac.TaxTotalType{
		TaxAmount:   amountValue(sum, true),
		TaxSubtotal: subs,
	}, sum, nil
}

func mergeDocumentTax(lines []cac.InvoiceLineType) (cac.TaxTotalType, Money, error) {
	totals := make([]*cac.TaxTotalType, 0, len(lines))
	for _, line := range lines {
		if line.TaxTotal != nil {
			totals = append(totals, line.TaxTotal)
		}
	}
	return mergeTaxTotals(totals)
}

func mergeWithholdingTax(lines []cac.InvoiceLineType) (cac.TaxTotalType, Money, error) {
	totals := make([]*cac.TaxTotalType, 0, len(lines))
	for _, line := range lines {
		for i := range line.WithholdingTaxTotal {
			totals = append(totals, &line.WithholdingTaxTotal[i])
		}
	}
	return mergeTaxTotals(totals)
}

func mergeCreditNoteTax(lines []cac.CreditNoteLineType) (cac.TaxTotalType, Money, error) {
	totals := make([]*cac.TaxTotalType, 0, len(lines))
	for _, line := range lines {
		for i := range line.TaxTotal {
			totals = append(totals, &line.TaxTotal[i])
		}
	}
	return mergeTaxTotals(totals)
}

func mergeTaxTotals(totals []*cac.TaxTotalType) (cac.TaxTotalType, Money, error) {
	type key struct{ code, percent, exemption string }
	type bucket struct {
		taxable         Money
		tax             Money
		name            string
		code            string
		percent         string
		exemptionCode   string
		exemptionReason string
	}
	order := []key{}
	by := map[key]*bucket{}

	for _, tot := range totals {
		if tot == nil {
			continue
		}
		for _, sub := range tot.TaxSubtotal {
			code := ""
			if sub.TaxCategory.TaxScheme.TaxTypeCode != nil {
				code = sub.TaxCategory.TaxScheme.TaxTypeCode.Value
			}
			percent := ""
			if sub.Percent != nil {
				percent = sub.Percent.Value
			}
			exemption := ""
			if sub.TaxCategory.TaxExemptionReasonCode != nil {
				exemption = sub.TaxCategory.TaxExemptionReasonCode.Value
			}
			k := key{code: code, percent: percent, exemption: exemption}
			bkt, ok := by[k]
			if !ok {
				cur := sub.TaxAmount.CurrencyID
				bkt = &bucket{
					taxable:       ZeroMoney(cur),
					tax:           ZeroMoney(cur),
					code:          code,
					percent:       percent,
					exemptionCode: exemption,
				}
				if sub.TaxCategory.TaxScheme.Name != nil {
					bkt.name = sub.TaxCategory.TaxScheme.Name.Value
				}
				if sub.TaxCategory.TaxExemptionReason != nil {
					bkt.exemptionReason = sub.TaxCategory.TaxExemptionReason.Value
				}
				by[k] = bkt
				order = append(order, k)
			}
			tax, err := parseAmount(sub.TaxAmount)
			if err != nil {
				return cac.TaxTotalType{}, Money{}, err
			}
			bkt.tax, err = bkt.tax.Add(tax)
			if err != nil {
				return cac.TaxTotalType{}, Money{}, err
			}
			if sub.TaxableAmount != nil {
				taxable, err := parseAmount(*sub.TaxableAmount)
				if err != nil {
					return cac.TaxTotalType{}, Money{}, err
				}
				bkt.taxable, err = bkt.taxable.Add(taxable)
				if err != nil {
					return cac.TaxTotalType{}, Money{}, err
				}
			}
		}
	}

	if len(order) == 0 {
		return cac.TaxTotalType{}, Money{}, nil
	}

	applied := make([]appliedTax, 0, len(order))
	for _, k := range order {
		bkt := by[k]
		applied = append(applied, appliedTax{
			tax:     bkt.tax.Round(),
			taxable: bkt.taxable.Round(),
			subtotal: cac.TaxSubtotalType{
				TaxableAmount: ptrAmount(bkt.taxable.Round()),
				TaxAmount:     amountValue(bkt.tax.Round(), true),
				Percent:       optPercent(bkt.percent),
				TaxCategory: cac.TaxCategoryType{
					TaxExemptionReasonCode: optExemptionCode(bkt.exemptionCode),
					TaxExemptionReason:     optExemptionReason(bkt.exemptionReason),
					TaxScheme: cac.TaxSchemeType{
						Name:        optText(bkt.name),
						TaxTypeCode: taxTypeCode(bkt.code),
					},
				},
			},
		})
	}
	return taxTotalOf(applied)
}

func emptyAmount(currency string) udt.AmountType {
	return amountValue(ZeroMoney(currency).Round(), true)
}

func defaultTaxName(code string) string {
	if code == codelist.TaxKDV {
		return "KDV"
	}
	return ""
}
