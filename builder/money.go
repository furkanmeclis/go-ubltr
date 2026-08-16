package builder

import (
	"errors"
	"fmt"
	"strings"

	"github.com/shopspring/decimal"
)

var (
	// ErrInvalidMoney is returned when an amount cannot be parsed.
	ErrInvalidMoney = errors.New("invalid money")
	// ErrCurrencyMismatch is returned when two amounts use different currencies.
	ErrCurrencyMismatch = errors.New("currency mismatch")
	// ErrInvalidRate is returned when a percent cannot be parsed.
	ErrInvalidRate = errors.New("invalid rate")
)

// Money is a decimal amount with an ISO 4217 currency. It is not a generated
// UBL type; builders write it onto udt.AmountType as a lexical string.
type Money struct {
	amount   decimal.Decimal
	currency string
}

// MoneyIn parses a decimal amount in currency. currency is stored upper-case.
func MoneyIn(currency, amount string) (Money, error) {
	currency = strings.ToUpper(strings.TrimSpace(currency))
	if currency == "" {
		return Money{}, fmt.Errorf("%w: empty currency", ErrInvalidMoney)
	}
	d, err := decimal.NewFromString(strings.TrimSpace(amount))
	if err != nil {
		return Money{}, fmt.Errorf("%w: %q", ErrInvalidMoney, amount)
	}
	return Money{amount: d, currency: currency}, nil
}

// ZeroMoney returns a zero amount in currency.
func ZeroMoney(currency string) Money {
	return Money{amount: decimal.Zero, currency: strings.ToUpper(strings.TrimSpace(currency))}
}

// Currency returns the ISO 4217 code.
func (m Money) Currency() string { return m.currency }

// String returns the shortest lexical decimal (unit prices keep extra scale).
func (m Money) String() string { return m.amount.String() }

// Lexical returns the amount rounded to the currency scale, with that many
// fraction digits (TRY totals are "17.88", not "17.880").
func (m Money) Lexical() string {
	return m.amount.Round(ScaleFor(m.currency)).StringFixed(ScaleFor(m.currency))
}

// Round returns m rounded to the currency's fraction digits.
func (m Money) Round() Money {
	m.amount = m.amount.Round(ScaleFor(m.currency))
	return m
}

// Add returns m + o. Currencies must match.
func (m Money) Add(o Money) (Money, error) {
	if err := m.sameCurrency(o); err != nil {
		return Money{}, err
	}
	return Money{amount: m.amount.Add(o.amount), currency: m.currency}, nil
}

// Sub returns m - o. Currencies must match.
func (m Money) Sub(o Money) (Money, error) {
	if err := m.sameCurrency(o); err != nil {
		return Money{}, err
	}
	return Money{amount: m.amount.Sub(o.amount), currency: m.currency}, nil
}

// TimesQuantity returns unit price × quantity. The currency is unchanged.
func (m Money) TimesQuantity(q Quantity) Money {
	return Money{amount: m.amount.Mul(q.value), currency: m.currency}
}

// PercentOf returns m × rate / 100 (for example 20 means 20%).
func (m Money) PercentOf(rate string) (Money, error) {
	p, err := decimal.NewFromString(strings.TrimSpace(rate))
	if err != nil {
		return Money{}, fmt.Errorf("%w: %q", ErrInvalidRate, rate)
	}
	hundred := decimal.NewFromInt(100)
	return Money{amount: m.amount.Mul(p).Div(hundred), currency: m.currency}, nil
}

// IsZero reports whether the amount is exactly zero.
func (m Money) IsZero() bool { return m.amount.IsZero() }

func (m Money) sameCurrency(o Money) error {
	if m.currency == "" || o.currency == "" {
		return fmt.Errorf("%w: empty currency", ErrInvalidMoney)
	}
	if m.currency != o.currency {
		return fmt.Errorf("%w: %s vs %s", ErrCurrencyMismatch, m.currency, o.currency)
	}
	return nil
}

// ScaleFor returns the ISO 4217 minor-unit digits for currency. Unknown codes use 2.
func ScaleFor(currency string) int32 {
	switch strings.ToUpper(strings.TrimSpace(currency)) {
	case "JPY", "KRW", "VND":
		return 0
	case "BHD", "IQD", "JOD", "KWD", "LYD", "OMR", "TND":
		return 3
	default:
		return 2
	}
}
