package codegen

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

var letRe = regexp.MustCompile(`name="([^"]+)"\s+value\s*=\s*"\'([^\']*)\'"`)

type codeListSpec struct {
	XMLName string
	Prefix  string
	Valid   string
	Consts  bool
}

// GİB UBL-TR_Codelist.xml entries. UBL XSD does not enumerate these codes.
var codeListSpecs = []codeListSpec{
	{XMLName: "ProfileIDType", Prefix: "Profile", Valid: "ValidProfile", Consts: true},
	{XMLName: "ProfileIDTypeEarchive", Prefix: "Profile", Valid: "ValidProfileEArchive", Consts: true},
	{XMLName: "ProfileIDTypeDespatchAdvice", Prefix: "Profile", Valid: "ValidProfileDespatch", Consts: true},
	{XMLName: "ProfileIDTypeGoruntuleme", Prefix: "Profile", Valid: "ValidProfileViewer", Consts: true},
	{XMLName: "InvoiceTypeCodeList", Prefix: "Invoice", Valid: "ValidInvoiceType", Consts: true},
	{XMLName: "DespatchAdviceTypeCodeList", Prefix: "Despatch", Valid: "ValidDespatchType", Consts: true},
	{XMLName: "ReceiptAdviceTypeCodeList", Prefix: "Receipt", Valid: "ValidReceiptType", Consts: true},
	{XMLName: "EnvelopeType", Prefix: "Envelope", Valid: "ValidEnvelopeType", Consts: true},
	{XMLName: "ElementType", Prefix: "Element", Valid: "ValidElementType", Consts: true},
	{XMLName: "TaxType", Prefix: "Tax", Valid: "ValidTax", Consts: true},
	{XMLName: "WithholdingTaxType", Prefix: "Withholding", Valid: "ValidWithholdingTax", Consts: true},
	{XMLName: "WithholdingTaxTypeWithPercent", Prefix: "WithholdingPercent", Valid: "ValidWithholdingPercent", Consts: true},
	{XMLName: "AccountingCostCodeList", Prefix: "AccountingCost", Valid: "ValidAccountingCost", Consts: true},
	{XMLName: "TaxExemptionReasonCodeType", Prefix: "Exemption", Valid: "ValidExemption", Consts: true},
	{XMLName: "YatirimTesvikTaxExemptionReasonCodeType", Prefix: "ExemptionYatirim", Valid: "ValidExemptionYatirim", Consts: true},
	{XMLName: "istisnaTaxExemptionReasonCodeType", Prefix: "ExemptionIstisna", Valid: "ValidExemptionIstisna", Consts: true},
	{XMLName: "ozelMatrahTaxExemptionReasonCodeType", Prefix: "ExemptionOzelMatrah", Valid: "ValidExemptionOzelMatrah", Consts: true},
	{XMLName: "ihracExemptionReasonCodeType", Prefix: "ExemptionIhrac", Valid: "ValidExemptionIhrac", Consts: true},
	{XMLName: "DeliveryTermCodeList", Prefix: "DeliveryTerm", Valid: "ValidDeliveryTerm", Consts: true},
	{XMLName: "TransportModeCodeList", Prefix: "TransportMode", Valid: "ValidTransportMode", Consts: true},
	{XMLName: "PartyIdentificationIDType", Prefix: "Scheme", Valid: "ValidPartyScheme", Consts: true},
	{XMLName: "LicensePlateIDSchemeIDType", Prefix: "PlateScheme", Valid: "ValidPlateScheme", Consts: true},
	{XMLName: "DocumentDescriptionType", Prefix: "DocumentDesc", Valid: "ValidDocumentDesc", Consts: true},
	{XMLName: "ResponseCodeType", Prefix: "Response", Valid: "ValidResponse", Consts: true},
	{XMLName: "AppResponseCodeType", Prefix: "AppResponse", Valid: "ValidAppResponse", Consts: true},
	{XMLName: "ContactTypeIdentifierType", Prefix: "ContactID", Valid: "ValidContactID", Consts: true},
	{XMLName: "PackageTypeCodeList", Prefix: "Package", Valid: "ValidPackage", Consts: true},
	{XMLName: "CurrencyCodeList", Prefix: "Currency", Valid: "ValidCurrency", Consts: true},
	{XMLName: "CountryCodeList", Prefix: "Country", Valid: "ValidCountry", Consts: true},
	{XMLName: "UserType", Prefix: "UserType", Valid: "ValidUserType", Consts: true},
	{XMLName: "ReservedAliases", Prefix: "Alias", Valid: "ValidAlias", Consts: true},
	{XMLName: "UserEnvelopeAliases", Prefix: "EnvelopeAlias", Valid: "ValidEnvelopeAlias", Consts: true},
	{XMLName: "UnitCodeList", Prefix: "Unit", Valid: "ValidUnit", Consts: true},
	{XMLName: "PaymentMeansCodeTypeList", Prefix: "PaymentMeans", Valid: "ValidPaymentMeans", Consts: true},
	{XMLName: "ChannelCodeList", Prefix: "Channel", Valid: "ValidChannel", Consts: true},
	{XMLName: "MimeCodeList", Prefix: "MIME", Valid: "ValidMIME", Consts: false},
	{XMLName: "AdditionalItemIdentificationIDType", Prefix: "ItemID", Valid: "ValidItemID", Consts: true},
	{XMLName: "IhracKayitliPartyIdentificationIDType", Prefix: "IhracScheme", Valid: "ValidIhracScheme", Consts: true},
	{XMLName: "YatirimTesvikEArsivInvoiceTypeCodeList", Prefix: "Invoice", Valid: "ValidYatirimTesvikInvoiceType", Consts: true},
	{XMLName: "YatirimTesvikItemClassificationCodeList", Prefix: "YatirimClass", Valid: "ValidYatirimClass", Consts: true},
}

// GenerateCodeLists writes GİB code-list constants and Valid* helpers from
// UBL-TR_Codelist.xml (the lists are not XSD enumerations).
func GenerateCodeLists(src, outDir string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("read codelist: %w", err)
	}
	parsed := parseLets(string(data))
	sum := sha256.Sum256(data)
	checksum := hex.EncodeToString(sum[:])

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}

	constBuf := bytes.NewBuffer(nil)
	lookupBuf := bytes.NewBuffer(nil)
	writeHeader(constBuf, checksum, true)
	writeHeader(lookupBuf, checksum, false)
	fmt.Fprintln(lookupBuf, "import \"strings\"")
	fmt.Fprintln(lookupBuf)

	used := map[string]string{}
	for _, spec := range codeListSpecs {
		codes := parsed[spec.XMLName]
		if len(codes) == 0 {
			return fmt.Errorf("codelist %s: no values in %s", spec.XMLName, src)
		}
		writeConstBlock(constBuf, spec, codes, used)
		writeLookup(lookupBuf, spec, codes)
	}

	if err := writeGo(filepath.Join(outDir, "generated.go"), constBuf.Bytes()); err != nil {
		return err
	}
	return writeGo(filepath.Join(outDir, "lookup.go"), lookupBuf.Bytes())
}

func writeHeader(buf *bytes.Buffer, checksum string, withChecksum bool) {
	fmt.Fprintln(buf, "// Code generated by ubltr-gen from UBL-TR_Codelist.xml. DO NOT EDIT.")
	fmt.Fprintln(buf)
	fmt.Fprintln(buf, "package codelist")
	fmt.Fprintln(buf)
	if withChecksum {
		fmt.Fprintln(buf, "// SourceChecksum is SHA-256 of schemas/ubltr-1.2.1/schematron/UBL-TR_Codelist.xml.")
		fmt.Fprintf(buf, "const SourceChecksum = %q\n\n", checksum)
	}
}

func writeConstBlock(buf *bytes.Buffer, spec codeListSpec, codes []string, used map[string]string) {
	if !spec.Consts {
		return
	}
	fmt.Fprintf(buf, "// %s values from GİB UBL-TR_Codelist.xml (%s).\n", spec.Prefix, spec.XMLName)
	fmt.Fprintln(buf, "const (")
	for _, code := range codes {
		ident, ok := constIdent(spec.Prefix, code)
		if !ok {
			continue
		}
		if prev, exists := used[ident]; exists {
			if prev != code {
				fmt.Fprintf(buf, "\t// skipped %s: identifier %s already used for %q\n", code, ident, prev)
			}
			continue
		}
		used[ident] = code
		fmt.Fprintf(buf, "\t%s = %q\n", ident, code)
	}
	fmt.Fprintln(buf, ")")
	fmt.Fprintln(buf)
}

func writeLookup(buf *bytes.Buffer, spec codeListSpec, codes []string) {
	mapName := unexport(spec.Valid) + "Set"
	fmt.Fprintf(buf, "// %s reports whether code is in GİB %s.\n", spec.Valid, spec.XMLName)
	fmt.Fprintf(buf, "func %s(code string) bool {\n", spec.Valid)
	fmt.Fprintf(buf, "\t_, ok := %s[strings.TrimSpace(code)]\n", mapName)
	fmt.Fprintln(buf, "\treturn ok")
	fmt.Fprintln(buf, "}")
	fmt.Fprintln(buf)
	fmt.Fprintf(buf, "var %s = map[string]struct{}{\n", mapName)
	for _, code := range codes {
		fmt.Fprintf(buf, "\t%q: {},\n", code)
	}
	fmt.Fprintln(buf, "}")
	fmt.Fprintln(buf)
}

func parseLets(src string) map[string][]string {
	out := map[string][]string{}
	for _, m := range letRe.FindAllStringSubmatch(src, -1) {
		name := m[1]
		raw := m[2]
		seen := map[string]struct{}{}
		var codes []string
		for _, p := range strings.Split(raw, ",") {
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
		out[name] = codes
	}
	return out
}

func constIdent(prefix, code string) (string, bool) {
	if strings.ContainsAny(code, "/+ \t@:") {
		return "", false
	}
	var b strings.Builder
	b.WriteString(prefix)
	for i, r := range code {
		switch {
		case r == '-' || r == '.' || r == '_':
			continue
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if i == 0 && unicode.IsLetter(r) {
				b.WriteRune(unicode.ToUpper(r))
				continue
			}
			b.WriteRune(r)
		default:
			return "", false
		}
	}
	ident := b.String()
	if ident == prefix {
		return "", false
	}
	if !unicode.IsLetter(rune(ident[0])) {
		return "", false
	}
	return ident, true
}

func unexport(s string) string {
	if s == "" {
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
}

func writeGo(path string, src []byte) error {
	formatted, err := format.Source(src)
	if err != nil {
		return fmt.Errorf("gofmt %s: %w\n%s", path, err, snippet(src))
	}
	return os.WriteFile(path, formatted, 0o644)
}

func snippet(src []byte) string {
	lines := bytes.Split(src, []byte("\n"))
	if len(lines) > 40 {
		lines = lines[:40]
	}
	return string(bytes.Join(lines, []byte("\n")))
}
