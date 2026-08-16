package ubltr_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/furkanmeclis/go-ubltr"
	"github.com/furkanmeclis/go-ubltr/schema/earchive"
	"github.com/furkanmeclis/go-ubltr/schema/ubltr121/invoice"
)

func TestUnmarshalTemelFatura(t *testing.T) {
	t.Parallel()

	data := readFixture(t, "TemelFaturaOrnegi.xml")
	var inv invoice.Invoice
	if err := ubltr.Unmarshal(data, &inv); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if inv.XMLName.Local != "Invoice" {
		t.Fatalf("XMLName.Local = %q, want Invoice", inv.XMLName.Local)
	}
	if inv.UBLVersionID.Value != "2.1" {
		t.Fatalf("UBLVersionID = %q, want 2.1", inv.UBLVersionID.Value)
	}
	if inv.CustomizationID.Value != "TR1.2" {
		t.Fatalf("CustomizationID = %q, want TR1.2", inv.CustomizationID.Value)
	}
	if inv.ProfileID.Value != "TEMELFATURA" {
		t.Fatalf("ProfileID = %q, want TEMELFATURA", inv.ProfileID.Value)
	}
	if inv.ID.Value != "GIB20090000000001" {
		t.Fatalf("ID = %q", inv.ID.Value)
	}
	if inv.InvoiceTypeCode.Value != "SATIS" {
		t.Fatalf("InvoiceTypeCode = %q, want SATIS", inv.InvoiceTypeCode.Value)
	}
	if inv.DocumentCurrencyCode.Value != "TRY" {
		t.Fatalf("DocumentCurrencyCode = %q, want TRY", inv.DocumentCurrencyCode.Value)
	}
	if len(inv.InvoiceLine) != 1 {
		t.Fatalf("InvoiceLine count = %d, want 1", len(inv.InvoiceLine))
	}
	if got := supplierVKN(inv); got != "1288331521" {
		t.Fatalf("supplier VKN = %q, want 1288331521", got)
	}
}

func TestMarshalRoundTrip(t *testing.T) {
	t.Parallel()

	data := readFixture(t, "TemelFaturaOrnegi.xml")
	var inv invoice.Invoice
	if err := ubltr.Unmarshal(data, &inv); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	out, err := ubltr.Marshal(&inv)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.HasPrefix(string(out), xml.Header) {
		t.Fatalf("missing xml declaration: %s", string(out[:min(80, len(out))]))
	}

	var again invoice.Invoice
	if err := ubltr.Unmarshal(out, &again); err != nil {
		t.Fatalf("re-unmarshal: %v", err)
	}
	if again.ID.Value != inv.ID.Value {
		t.Fatalf("round-trip ID = %q, want %q", again.ID.Value, inv.ID.Value)
	}
	if again.ProfileID.Value != inv.ProfileID.Value {
		t.Fatalf("round-trip ProfileID = %q", again.ProfileID.Value)
	}
	if len(again.InvoiceLine) != len(inv.InvoiceLine) {
		t.Fatalf("round-trip lines = %d, want %d", len(again.InvoiceLine), len(inv.InvoiceLine))
	}
}

func TestUnmarshalInvalidXML(t *testing.T) {
	t.Parallel()

	var inv invoice.Invoice
	err := ubltr.Unmarshal([]byte(`<not-xml`), &inv)
	if err == nil {
		t.Fatal("expected error for invalid xml")
	}
}

func TestValidateXSD(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		xml     []byte
		wantErr bool
	}{
		{
			name:    "temel fatura fixture",
			xml:     readFixture(t, "TemelFaturaOrnegi.xml"),
			wantErr: false,
		},
		{
			name: "empty invoice",
			xml: []byte(`<?xml version="1.0" encoding="UTF-8"?>
<Invoice xmlns="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2"/>
`),
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := ubltr.ValidateXSD(t.Context(), tc.xml)
			if (err != nil) != tc.wantErr {
				t.Fatalf("ValidateXSD() error = %v, wantErr %v", err, tc.wantErr)
			}
			if tc.wantErr && err != nil {
				var ve *ubltr.XSDValidationError
				if !errors.As(err, &ve) {
					t.Fatalf("error type %T, want *ubltr.XSDValidationError", err)
				}
			}
		})
	}
}

func TestValidateSchematron(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		xml     []byte
		docType string
		wantErr bool
	}{
		{
			name:    "temel fatura fixture fails current rules",
			xml:     readFixture(t, "TemelFaturaOrnegi.xml"),
			docType: ubltr.TypeEFatura,
			wantErr: true,
		},
		{
			name:    "earchive rejects TEMELFATURA",
			xml:     readFixture(t, "TemelFaturaOrnegi.xml"),
			docType: ubltr.TypeEArchive,
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := ubltr.ValidateSchematronType(t.Context(), tc.xml, tc.docType)
			if (err != nil) != tc.wantErr {
				t.Fatalf("ValidateSchematronType() error = %v, wantErr %v", err, tc.wantErr)
			}
			if tc.wantErr && err != nil {
				var ve *ubltr.SchematronValidationError
				if !errors.As(err, &ve) {
					t.Fatalf("error type %T, want *ubltr.SchematronValidationError", err)
				}
			}
		})
	}
}

func TestValidateRunsXSDThenSchematron(t *testing.T) {
	t.Parallel()

	err := ubltr.Validate(t.Context(), readFixture(t, "TemelFaturaOrnegi.xml"))
	if err == nil {
		t.Fatal("expected schematron failure after xsd success")
	}
	var xe *ubltr.XSDValidationError
	if errors.As(err, &xe) {
		t.Fatalf("got xsd error, want schematron: %v", err)
	}
	var se *ubltr.SchematronValidationError
	if !errors.As(err, &se) {
		t.Fatalf("error type %T, want *ubltr.SchematronValidationError", err)
	}
}

func TestNewInvoice(t *testing.T) {
	t.Parallel()

	inv := ubltr.NewInvoice()
	if inv == nil {
		t.Fatal("NewInvoice returned nil")
	}
	out, err := ubltr.Marshal(inv)
	if err != nil {
		t.Fatalf("marshal empty invoice: %v", err)
	}
	if !strings.Contains(string(out), "Invoice") {
		t.Fatalf("marshaled xml missing Invoice: %s", out)
	}
}

func TestXSDValidationErrorHasPath(t *testing.T) {
	t.Parallel()

	err := ubltr.ValidateXSD(t.Context(), []byte(`<?xml version="1.0" encoding="UTF-8"?>
<Invoice xmlns="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2"/>
`))
	if err == nil {
		t.Fatal("expected xsd error")
	}
	var xe *ubltr.XSDValidationError
	if !errors.As(err, &xe) {
		t.Fatalf("type %T", err)
	}
	if len(xe.Violations) == 0 {
		t.Fatal("expected violations")
	}
	if xe.Violations[0].Layer != "xsd" {
		t.Fatalf("layer = %q", xe.Violations[0].Layer)
	}
	if xe.Violations[0].Message == "" && xe.Violations[0].Path == "" {
		t.Fatalf("empty violation: %+v", xe.Violations[0])
	}
	if len(xe.Messages) == 0 {
		t.Fatal("expected Messages for compatibility")
	}
}

func TestSchematronValidationErrorHasRuleID(t *testing.T) {
	t.Parallel()

	err := ubltr.ValidateSchematron(t.Context(), readFixture(t, "TemelFaturaOrnegi.xml"))
	if err == nil {
		t.Fatal("expected schematron error")
	}
	var se *ubltr.SchematronValidationError
	if !errors.As(err, &se) {
		t.Fatalf("type %T", err)
	}
	found := false
	for _, v := range se.Violations {
		if v.RuleID != "" || v.Path != "" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected path or rule id in %+v", se.Violations)
	}
}

func TestDetect(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		xml  []byte
		kind ubltr.Kind
	}{
		{
			name: "invoice fixture",
			xml:  readFixture(t, "TemelFaturaOrnegi.xml"),
			kind: ubltr.KindInvoice,
		},
		{
			name: "credit note root",
			xml:  []byte(`<CreditNote xmlns="urn:oasis:names:specification:ubl:schema:xsd:CreditNote-2"/>`),
			kind: ubltr.KindCreditNote,
		},
		{
			name: "application response root",
			xml:  []byte(`<ApplicationResponse xmlns="urn:oasis:names:specification:ubl:schema:xsd:ApplicationResponse-2"/>`),
			kind: ubltr.KindApplicationResponse,
		},
		{
			name: "despatch advice root",
			xml:  []byte(`<DespatchAdvice xmlns="urn:oasis:names:specification:ubl:schema:xsd:DespatchAdvice-2"/>`),
			kind: ubltr.KindDespatchAdvice,
		},
		{
			name: "receipt advice root",
			xml:  []byte(`<ReceiptAdvice xmlns="urn:oasis:names:specification:ubl:schema:xsd:ReceiptAdvice-2"/>`),
			kind: ubltr.KindReceiptAdvice,
		},
		{
			name: "e-archive report root",
			xml:  []byte(`<eArsivRaporu xmlns="http://earsiv.efatura.gov.tr"/>`),
			kind: ubltr.KindEArchiveReport,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := ubltr.Detect(tc.xml)
			if err != nil {
				t.Fatalf("Detect: %v", err)
			}
			if got != tc.kind {
				t.Fatalf("kind = %q, want %q", got, tc.kind)
			}
		})
	}
}

func TestDetectUnknown(t *testing.T) {
	t.Parallel()

	_, err := ubltr.Detect([]byte(`<Foo xmlns="urn:example:foo"/>`))
	if !errors.Is(err, ubltr.ErrUnknownDocument) {
		t.Fatalf("error = %v, want ErrUnknownDocument", err)
	}
}

func TestNewCreditNoteFromInvoice(t *testing.T) {
	t.Parallel()

	var inv invoice.Invoice
	if err := ubltr.Unmarshal(readFixture(t, "TemelFaturaOrnegi.xml"), &inv); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	cn, err := ubltr.NewCreditNoteFromInvoice(&inv)
	if err != nil {
		t.Fatalf("NewCreditNoteFromInvoice: %v", err)
	}
	if cn.ID.Value != inv.ID.Value {
		t.Fatalf("ID = %q", cn.ID.Value)
	}
	if len(cn.CreditNoteLine) != len(inv.InvoiceLine) {
		t.Fatalf("lines = %d", len(cn.CreditNoteLine))
	}
	if len(cn.BillingReference) == 0 || cn.BillingReference[len(cn.BillingReference)-1].InvoiceDocumentReference == nil {
		t.Fatal("expected invoice billing reference")
	}
	if cn.UUID == nil || cn.UUID.Value == "" || cn.UUID.Value == inv.UUID.Value {
		t.Fatal("expected a new UUID")
	}

	data, err := ubltr.Marshal(cn)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	kind, err := ubltr.Detect(data)
	if err != nil {
		t.Fatal(err)
	}
	if kind != ubltr.KindCreditNote {
		t.Fatalf("kind = %q", kind)
	}
	if err := ubltr.ValidateXSD(t.Context(), data); err != nil {
		t.Fatalf("ValidateXSD: %v\n%s", err, data)
	}
	if err := ubltr.Validate(t.Context(), data); err != nil {
		t.Fatalf("Validate (xsd only for credit note): %v", err)
	}
}

func TestNewConstructors(t *testing.T) {
	t.Parallel()

	if ubltr.NewCreditNote() == nil || ubltr.NewApplicationResponse() == nil ||
		ubltr.NewDespatchAdvice() == nil || ubltr.NewReceiptAdvice() == nil ||
		ubltr.NewEArchiveReport() == nil {
		t.Fatal("constructor returned nil")
	}
}

func supplierVKN(inv invoice.Invoice) string {
	for _, id := range inv.AccountingSupplierParty.Party.PartyIdentification {
		if id.ID.SchemeID == "VKN" {
			return id.ID.Value
		}
	}
	return ""
}

func TestEArchiveReportDetectAndEmptyXSD(t *testing.T) {
	t.Parallel()

	xml := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<eArsivRaporu xmlns="http://earsiv.efatura.gov.tr"/>
`)
	kind, err := ubltr.Detect(xml)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if kind != ubltr.KindEArchiveReport {
		t.Fatalf("kind = %q, want %q", kind, ubltr.KindEArchiveReport)
	}

	err = ubltr.ValidateXSD(t.Context(), xml)
	if err == nil {
		t.Fatal("expected empty e-Arşiv report to fail XSD")
	}
	var ve *ubltr.XSDValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("error type %T, want *ubltr.XSDValidationError: %v", err, err)
	}
}

func TestEArchiveReportTypes(t *testing.T) {
	t.Parallel()

	if earchive.FaturaTipYTBTEVKIFAT != "YTBTEVKIFAT" {
		t.Fatalf("FaturaTipYTBTEVKIFAT = %q", earchive.FaturaTipYTBTEVKIFAT)
	}
	if earchive.FaturaTipYTBTEVKIFATIADE != "YTBTEVKIFATIADE" {
		t.Fatalf("FaturaTipYTBTEVKIFATIADE = %q", earchive.FaturaTipYTBTEVKIFATIADE)
	}

	sum, err := sha256File(t, filepath.Join("schemas", "earchive", "EArsiv.xsd"))
	if err != nil {
		t.Fatal(err)
	}
	if sum != earchive.ChecksumEArsiv {
		t.Fatalf("ChecksumEArsiv = %s, file = %s", earchive.ChecksumEArsiv, sum)
	}

	rep := ubltr.NewEArchiveReport()
	rep.Baslik.Versiyon = "1.0"
	data, err := ubltr.Marshal(rep)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	kind, err := ubltr.Detect(data)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if kind != ubltr.KindEArchiveReport {
		t.Fatalf("kind = %q", kind)
	}
}

func sha256File(t *testing.T, path string) (string, error) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	path := filepath.Join("testdata", "ubltr-1.2.1", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return data
}
