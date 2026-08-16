package builder

import (
	"errors"
	"fmt"
	"strings"

	"github.com/shopspring/decimal"
)

// ErrInvalidQuantity is returned when a quantity cannot be parsed.
var ErrInvalidQuantity = errors.New("invalid quantity")

// Quantity is a decimal count with a UNECE unit code. Builders write it onto
// udt.QuantityType as a lexical string.
type Quantity struct {
	value decimal.Decimal
	unit  string
}

// QuantityIn parses value with a unit code (for example C62, KWH, KGM).
func QuantityIn(unit, value string) (Quantity, error) {
	unit = strings.TrimSpace(unit)
	if unit == "" {
		return Quantity{}, fmt.Errorf("%w: empty unit", ErrInvalidQuantity)
	}
	d, err := decimal.NewFromString(strings.TrimSpace(value))
	if err != nil {
		return Quantity{}, fmt.Errorf("%w: %q", ErrInvalidQuantity, value)
	}
	return Quantity{value: d, unit: unit}, nil
}

// Unit returns the unit code.
func (q Quantity) Unit() string { return q.unit }

// String returns the lexical decimal.
func (q Quantity) String() string { return q.value.String() }
