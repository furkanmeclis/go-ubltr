package builder

import (
	"math/big"
	"strings"

	"github.com/shopspring/decimal"
)

// currencyWords holds the Turkish major and minor unit names for a currency.
type currencyWords struct {
	major, minor string
}

var currencyNames = map[string]currencyWords{
	"TRY": {"TÜRK LİRASI", "KURUŞ"},
	"USD": {"ABD DOLARI", "SENT"},
	"EUR": {"AVRO", "SENT"},
	"GBP": {"İNGİLİZ STERLİNİ", "PENİ"},
	"CHF": {"İSVİÇRE FRANGI", "SANTİM"},
	"JPY": {"JAPON YENİ", "SEN"},
	"RUB": {"RUS RUBLESİ", "KAPİK"},
	"CNY": {"ÇİN YUANI", "FEN"},
	"SAR": {"SUUDİ ARABİSTAN RİYALİ", "HALALA"},
	"AED": {"BAE DİRHEMİ", "FİLS"},
	"KWD": {"KUVEYT DİNARI", "FİLS"},
}

var (
	wordOnes   = [...]string{"", "BİR", "İKİ", "ÜÇ", "DÖRT", "BEŞ", "ALTI", "YEDİ", "SEKİZ", "DOKUZ"}
	wordTens   = [...]string{"", "ON", "YİRMİ", "OTUZ", "KIRK", "ELLİ", "ALTMIŞ", "YETMİŞ", "SEKSEN", "DOKSAN"}
	wordScales = [...]string{"", "BİN", "MİLYON", "MİLYAR", "TRİLYON", "KATRİLYON", "KENTİLYON"}
)

// AmountInWords writes m in Turkish words in the form printed on GİB
// invoices, for example 123.45 TRY is "YALNIZ YÜZ YİRMİ ÜÇ TÜRK LİRASI KIRK
// BEŞ KURUŞ". The amount is rounded to the currency scale ([ScaleFor]); a zero
// fraction is omitted. TRY, USD, EUR, GBP, CHF, JPY, RUB, CNY, SAR, AED, and
// KWD have Turkish unit names; other currencies use the ISO code as the major
// unit and "KÜSUR" as the minor unit. Negative amounts start with "EKSİ".
func AmountInWords(m Money) string {
	scale := ScaleFor(m.currency)
	amt := m.amount.Round(scale)
	neg := amt.IsNegative()
	amt = amt.Abs()

	whole := amt.Truncate(0)
	frac := amt.Sub(whole).Shift(scale).Truncate(0)

	names, ok := currencyNames[m.currency]
	if !ok {
		names = currencyWords{major: m.currency, minor: "KÜSUR"}
	}

	parts := []string{"YALNIZ"}
	if neg && !amt.IsZero() {
		parts = append(parts, "EKSİ")
	}
	parts = append(parts, integerWords(whole.BigInt()), names.major)
	if scale > 0 && !frac.IsZero() {
		parts = append(parts, integerWords(frac.BigInt()), names.minor)
	}
	return strings.Join(strings.Fields(strings.Join(parts, " ")), " ")
}

// integerWords spells a non-negative integer in Turkish. Zero is "SIFIR".
// One thousand is "BİN", not "BİR BİN"; one hundred is "YÜZ".
func integerWords(n *big.Int) string {
	if n.Sign() == 0 {
		return "SIFIR"
	}
	thousand := big.NewInt(1000)
	groups := []int64{}
	rest := new(big.Int).Set(n)
	mod := new(big.Int)
	for rest.Sign() > 0 {
		rest.DivMod(rest, thousand, mod)
		groups = append(groups, mod.Int64())
	}
	if len(groups) > len(wordScales) {
		// Beyond the named scales: spell digit groups without a scale word.
		return strings.TrimSpace(decimal.NewFromBigInt(n, 0).String())
	}

	words := []string{}
	for i := len(groups) - 1; i >= 0; i-- {
		g := groups[i]
		if g == 0 {
			continue
		}
		if i == 1 && g == 1 {
			words = append(words, wordScales[1])
			continue
		}
		words = append(words, groupWords(g))
		if wordScales[i] != "" {
			words = append(words, wordScales[i])
		}
	}
	return strings.Join(words, " ")
}

func groupWords(g int64) string {
	h, t, o := g/100, (g/10)%10, g%10
	words := []string{}
	switch {
	case h == 1:
		words = append(words, "YÜZ")
	case h > 1:
		words = append(words, wordOnes[h], "YÜZ")
	}
	if t > 0 {
		words = append(words, wordTens[t])
	}
	if o > 0 {
		words = append(words, wordOnes[o])
	}
	return strings.Join(words, " ")
}
