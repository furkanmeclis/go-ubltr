package builder_test

import (
	"errors"
	"testing"

	"github.com/furkanmeclis/go-ubltr/builder"
	"github.com/furkanmeclis/go-ubltr/codelist"
)

func TestMoneyIn(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, currency, amount string
		want                   string
		wantErr                error
	}{
		{name: "try", currency: "try", amount: "17.88", want: "17.88"},
		{name: "extra scale", currency: "TRY", amount: "0.15", want: "0.15"},
		{name: "empty currency", currency: "", amount: "1", wantErr: builder.ErrInvalidMoney},
		{name: "bad amount", currency: "TRY", amount: "x", wantErr: builder.ErrInvalidMoney},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := builder.MoneyIn(tc.currency, tc.amount)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("error = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("MoneyIn: %v", err)
			}
			if got.Currency() != "TRY" {
				t.Fatalf("currency = %q", got.Currency())
			}
			if got.String() != tc.want {
				t.Fatalf("string = %q, want %q", got.String(), tc.want)
			}
		})
	}
}

func TestMoneyAddSubAndMismatch(t *testing.T) {
	t.Parallel()

	a, err := builder.MoneyIn(codelist.TRY, "10.00")
	if err != nil {
		t.Fatal(err)
	}
	b, err := builder.MoneyIn(codelist.TRY, "2.50")
	if err != nil {
		t.Fatal(err)
	}
	sum, err := a.Add(b)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Lexical() != "12.50" {
		t.Fatalf("sum = %q", sum.Lexical())
	}
	diff, err := a.Sub(b)
	if err != nil {
		t.Fatal(err)
	}
	if diff.Lexical() != "7.50" {
		t.Fatalf("diff = %q", diff.Lexical())
	}

	usd, err := builder.MoneyIn(codelist.USD, "1.00")
	if err != nil {
		t.Fatal(err)
	}
	_, err = a.Add(usd)
	if !errors.Is(err, builder.ErrCurrencyMismatch) {
		t.Fatalf("error = %v, want ErrCurrencyMismatch", err)
	}
}

func TestMoneyPercentAndLineExtension(t *testing.T) {
	t.Parallel()

	price, err := builder.MoneyIn(codelist.TRY, "0.15")
	if err != nil {
		t.Fatal(err)
	}
	qty, err := builder.QuantityIn(codelist.UnitKWH, "101")
	if err != nil {
		t.Fatal(err)
	}
	ext := price.TimesQuantity(qty).Round()
	if ext.Lexical() != "15.15" {
		t.Fatalf("extension = %q, want 15.15", ext.Lexical())
	}
	tax, err := ext.PercentOf("18")
	if err != nil {
		t.Fatal(err)
	}
	if tax.Round().Lexical() != "2.73" {
		t.Fatalf("tax = %q, want 2.73", tax.Round().Lexical())
	}
}

func TestQuantityIn(t *testing.T) {
	t.Parallel()

	q, err := builder.QuantityIn(codelist.UnitC62, "3")
	if err != nil {
		t.Fatal(err)
	}
	if q.Unit() != codelist.UnitC62 || q.String() != "3" {
		t.Fatalf("quantity = %+v", q)
	}
	_, err = builder.QuantityIn("", "1")
	if !errors.Is(err, builder.ErrInvalidQuantity) {
		t.Fatalf("error = %v", err)
	}
}

func TestScaleFor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		code string
		want int32
	}{
		{code: "TRY", want: 2},
		{code: "JPY", want: 0},
		{code: "KWD", want: 3},
		{code: "XXX", want: 2},
	}
	for _, tc := range tests {
		t.Run(tc.code, func(t *testing.T) {
			t.Parallel()
			if got := builder.ScaleFor(tc.code); got != tc.want {
				t.Fatalf("ScaleFor(%s) = %d, want %d", tc.code, got, tc.want)
			}
		})
	}
}
