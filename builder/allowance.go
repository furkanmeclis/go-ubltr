package builder

import (
	"github.com/furkanmeclis/go-ubltr/schema/ubltr121/cac"
	"github.com/furkanmeclis/go-ubltr/schema/ubltr121/cbc"
	"github.com/shopspring/decimal"
)

type allowanceResult struct {
	xml        []cac.AllowanceChargeType
	extension  Money
	allowances Money
	charges    Money
}

func applyAllowances(base Money, specs []allowanceSpec) (allowanceResult, error) {
	out := allowanceResult{
		xml:        []cac.AllowanceChargeType{},
		extension:  base,
		allowances: ZeroMoney(base.currency),
		charges:    ZeroMoney(base.currency),
	}
	if len(specs) == 0 {
		return out, nil
	}

	for _, spec := range specs {
		amt, factor, err := resolveAllowance(base, spec)
		if err != nil {
			return allowanceResult{}, err
		}
		if spec.charge {
			out.extension, err = out.extension.Add(amt)
			if err != nil {
				return allowanceResult{}, builderWrap("charge", err)
			}
			out.charges, err = out.charges.Add(amt)
			if err != nil {
				return allowanceResult{}, builderWrap("charge", err)
			}
		} else {
			out.extension, err = out.extension.Sub(amt)
			if err != nil {
				return allowanceResult{}, builderWrap("allowance", err)
			}
			out.allowances, err = out.allowances.Add(amt)
			if err != nil {
				return allowanceResult{}, builderWrap("allowance", err)
			}
		}

		ac := cac.AllowanceChargeType{
			ChargeIndicator:         cbc.ChargeIndicatorType{Value: boolString(spec.charge)},
			AllowanceChargeReason:   optNote(spec.reason),
			MultiplierFactorNumeric: optMultiplier(factor),
			Amount:                  amountValue(amt.Round(), true),
			BaseAmount:              ptrBase(base.Round()),
		}
		out.xml = append(out.xml, ac)
	}
	return out, nil
}

func resolveAllowance(base Money, spec allowanceSpec) (Money, string, error) {
	if spec.hasAmt {
		if err := base.sameCurrency(spec.amount); err != nil {
			return Money{}, "", builderWrap("allowance", err)
		}
		return spec.amount, "", nil
	}
	if spec.percent == "" {
		return Money{}, "", builderErr("allowance", "amount or percent is required")
	}
	amt, err := base.PercentOf(spec.percent)
	if err != nil {
		return Money{}, "", builderWrap("allowance.percent", err)
	}
	p, err := decimal.NewFromString(spec.percent)
	if err != nil {
		return Money{}, "", builderWrap("allowance.percent", err)
	}
	factor := p.Div(decimal.NewFromInt(100)).String()
	return amt, factor, nil
}

func boolString(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

func ptrBase(m Money) *cbc.BaseAmountType {
	a := amountValue(m, true)
	return &a
}
