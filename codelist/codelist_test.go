package codelist_test

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/furkanmeclis/go-ubltr/codelist"
)

var letRe = regexp.MustCompile(`name="([^"]+)"\s+value\s*=\s*"\'([^\']*)\'"`)

func TestGeneratedListsMatchXML(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("../schemas/ubltr-1.2.1/schematron/UBL-TR_Codelist.xml")
	if err != nil {
		t.Fatal(err)
	}
	lists := map[string][]string{}
	for _, m := range letRe.FindAllStringSubmatch(string(data), -1) {
		var codes []string
		seen := map[string]struct{}{}
		for _, p := range strings.Split(m[2], ",") {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			if _, ok := seen[p]; ok {
				continue
			}
			seen[p] = struct{}{}
			codes = append(codes, p)
		}
		lists[m[1]] = codes
	}

	tests := []struct {
		name  string
		xml   string
		valid func(string) bool
	}{
		{"unit", "UnitCodeList", codelist.ValidUnit},
		{"currency", "CurrencyCodeList", codelist.ValidCurrency},
		{"country", "CountryCodeList", codelist.ValidCountry},
		{"profile", "ProfileIDType", codelist.ValidProfile},
		{"invoice type", "InvoiceTypeCodeList", codelist.ValidInvoiceType},
		{"tax", "TaxType", codelist.ValidTax},
		{"payment means", "PaymentMeansCodeTypeList", codelist.ValidPaymentMeans},
		{"mime", "MimeCodeList", codelist.ValidMIME},
		{"package", "PackageTypeCodeList", codelist.ValidPackage},
		{"party scheme", "PartyIdentificationIDType", codelist.ValidPartyScheme},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			codes := lists[tc.xml]
			if len(codes) == 0 {
				t.Fatalf("xml list %s empty", tc.xml)
			}
			for _, code := range codes {
				if !tc.valid(code) {
					t.Fatalf("%s missing %q", tc.xml, code)
				}
			}
			if tc.valid("___not_a_code___") {
				t.Fatal("expected unknown code to be invalid")
			}
		})
	}
}

func TestCommonConstants(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, got, want string
	}{
		{"unit C62", codelist.UnitC62, "C62"},
		{"unit KWH", codelist.UnitKWH, "KWH"},
		{"currency TRY", codelist.CurrencyTRY, "TRY"},
		{"TRY alias", codelist.TRY, "TRY"},
		{"profile", codelist.ProfileTEMELFATURA, "TEMELFATURA"},
		{"invoice", codelist.InvoiceSATIS, "SATIS"},
		{"tax KDV", codelist.Tax0015, "0015"},
		{"tax alias", codelist.TaxKDV, "0015"},
		{"scheme VKN", codelist.SchemeVKN, "VKN"},
		{"payment 30", codelist.PaymentMeans30, "30"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if tc.got != tc.want {
				t.Fatalf("got %q, want %q", tc.got, tc.want)
			}
		})
	}
	if !codelist.ValidUnit(codelist.UnitC62) || !codelist.ValidCurrency(codelist.TRY) {
		t.Fatal("common constants must pass Valid*")
	}
}
