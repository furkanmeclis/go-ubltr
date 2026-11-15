package ubltr_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/furkanmeclis/go-ubltr"
	"github.com/furkanmeclis/go-ubltr/builder"
	"github.com/furkanmeclis/go-ubltr/codelist"
	"github.com/furkanmeclis/go-ubltr/schema/ubltr121/applicationresponse"
	"github.com/furkanmeclis/go-ubltr/schema/ubltr121/cac"
	"github.com/furkanmeclis/go-ubltr/schema/ubltr121/cbc"
	"github.com/furkanmeclis/go-ubltr/schema/ubltr121/despatchadvice"
	"github.com/furkanmeclis/go-ubltr/schema/ubltr121/invoice"
)

func TestSchematronViolationFields(t *testing.T) {
	t.Parallel()

	err := ubltr.ValidateSchematron(t.Context(), readFixture(t, "TemelFaturaOrnegi.xml"))
	var se *ubltr.SchematronValidationError
	if !errors.As(err, &se) {
		t.Fatalf("error %T %v, want *SchematronValidationError", err, err)
	}
	var tckn *ubltr.Violation
	for i, v := range se.Violations {
		if v.RuleID == "PartyIdentificationTCKNVKNCheck" {
			tckn = &se.Violations[i]
			break
		}
	}
	if tckn == nil {
		t.Fatalf("no PartyIdentificationTCKNVKNCheck in %+v", se.Violations)
	}
	checks := []struct {
		name, got, want string
	}{
		{"Layer", tckn.Layer, ubltr.LayerSchematron},
		{"Severity", tckn.Severity, ubltr.SeverityError},
		{"PatternID", tckn.PatternID, "invoice"},
		{"Context", tckn.Context, "inv:Invoice/cac:AccountingCustomerParty/cac:Party/cac:PartyIdentification"},
		{"Element", tckn.Element, "cac:PartyIdentification"},
		{"Path", tckn.Path, "cac:PartyIdentification"},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %q, want %q", c.name, c.got, c.want)
		}
	}
	if !strings.HasPrefix(tckn.Location, "/inv:Invoice/cac:AccountingCustomerParty/cac:Party/cac:PartyIdentification") {
		t.Errorf("Location = %q", tckn.Location)
	}
	if tckn.Line <= 0 {
		t.Errorf("Line = %d, want > 0", tckn.Line)
	}
	if !strings.Contains(tckn.Test, "string-length") {
		t.Errorf("Test = %q", tckn.Test)
	}
	if !strings.Contains(tckn.String(), tckn.Location) {
		t.Errorf("String() = %q, want location", tckn.String())
	}
	if len(se.Errors()) != len(se.Violations) || len(se.Warnings()) != 0 {
		t.Errorf("Errors/Warnings split = %d/%d of %d", len(se.Errors()), len(se.Warnings()), len(se.Violations))
	}
}

func TestXSDViolationFields(t *testing.T) {
	t.Parallel()

	data := readFixture(t, "TemelFaturaOrnegi.xml")
	bad := strings.Replace(string(data), "<cbc:ProfileID>", "<cbc:Unexpected>x</cbc:Unexpected><cbc:ProfileID>", 1)
	err := ubltr.ValidateXSD(t.Context(), []byte(bad))
	var xe *ubltr.XSDValidationError
	if !errors.As(err, &xe) {
		t.Fatalf("error %T %v, want *XSDValidationError", err, err)
	}
	v := xe.Violations[0]
	if v.Layer != ubltr.LayerXSD || v.Severity != ubltr.SeverityError {
		t.Fatalf("layer/severity = %q/%q", v.Layer, v.Severity)
	}
	if v.Line <= 0 {
		t.Fatalf("Line = %d", v.Line)
	}
	if v.Element != "cbc:Unexpected" {
		t.Fatalf("Element = %q", v.Element)
	}
	if v.Location != "/inv:Invoice/cbc:Unexpected" {
		t.Fatalf("Location = %q", v.Location)
	}
	if len(xe.Errors()) == 0 {
		t.Fatal("Errors() is empty")
	}
}

func TestViolationsHelper(t *testing.T) {
	t.Parallel()

	err := ubltr.ValidateSchematron(t.Context(), readFixture(t, "TemelFaturaOrnegi.xml"))
	wrapped := fmt.Errorf("upload: %w", err)
	got := ubltr.Violations(wrapped)
	if len(got) == 0 {
		t.Fatal("Violations(wrapped) is empty")
	}
	var se *ubltr.SchematronValidationError
	if !errors.As(wrapped, &se) || len(se.Violations) != len(got) {
		t.Fatalf("errors.As through wrap: %v", wrapped)
	}
	if ubltr.Violations(errors.New("other")) != nil {
		t.Fatal("Violations(other) should be nil")
	}
	if ubltr.Violations(nil) != nil {
		t.Fatal("Violations(nil) should be nil")
	}
}

func TestHasSchematron(t *testing.T) {
	t.Parallel()

	tests := []struct {
		kind ubltr.Kind
		want bool
	}{
		{ubltr.KindInvoice, true},
		{ubltr.KindApplicationResponse, true},
		{ubltr.KindDespatchAdvice, true},
		{ubltr.KindReceiptAdvice, true},
		{ubltr.KindEnvelope, true},
		{ubltr.KindCreditNote, false},
		{ubltr.KindEArchiveReport, false},
	}
	for _, tc := range tests {
		t.Run(string(tc.kind), func(t *testing.T) {
			t.Parallel()
			if got := ubltr.HasSchematron(tc.kind); got != tc.want {
				t.Fatalf("HasSchematron(%s) = %v, want %v", tc.kind, got, tc.want)
			}
		})
	}
}

func TestValidateOtherDocuments(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		doc      func(t *testing.T) any
		wantRule string // empty: valid
		pattern  string
	}{
		{
			name: "despatch advice valid",
			doc:  func(t *testing.T) any { return validDespatch(t) },
		},
		{
			name: "despatch advice with driver but without plate",
			doc: func(t *testing.T) any {
				d := validDespatch(t)
				d.Shipment.ShipmentStage[0].TransportMeans = nil
				return d
			},
			wantRule: "LicensePlateIDCheck",
			pattern:  "despatchadvice",
		},
		{
			name: "despatch advice without despatch date",
			doc: func(t *testing.T) any {
				d := validDespatch(t)
				d.Shipment.Delivery.Despatch = nil
				return d
			},
			wantRule: "DespatchDateCheck",
			pattern:  "despatchadvice",
		},
		{
			name: "application response valid",
			doc:  func(t *testing.T) any { return validApplicationResponse(t) },
		},
		{
			name: "application response with invoice profile",
			doc: func(t *testing.T) any {
				ar := validApplicationResponse(t)
				ar.ProfileID.Value = codelist.ProfileTemelFatura
				return ar
			},
			wantRule: "ApplicationResponseProfileIDCheck",
			pattern:  "applicationresponse",
		},
		{
			name: "receipt advice valid",
			doc:  func(t *testing.T) any { return validReceipt(t) },
		},
		{
			name: "invoice valid",
			doc:  func(t *testing.T) any { return validInvoice(t) },
		},
		{
			name: "invoice with bad unit code",
			doc: func(t *testing.T) any {
				inv := validInvoice(t)
				inv.InvoiceLine[0].InvoicedQuantity.UnitCode = "NOPE"
				return inv
			},
			wantRule: "GeneralUnitCodeCheck",
			pattern:  "general",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			data, err := ubltr.Marshal(tc.doc(t))
			if err != nil {
				t.Fatal(err)
			}
			err = ubltr.Validate(t.Context(), data)
			if tc.wantRule == "" {
				if err != nil {
					for _, v := range ubltr.Violations(err) {
						t.Log(v.String())
					}
					t.Fatalf("Validate: %v", err)
				}
				return
			}
			var se *ubltr.SchematronValidationError
			if !errors.As(err, &se) {
				t.Fatalf("error %T %v, want *SchematronValidationError", err, err)
			}
			for _, v := range se.Violations {
				if v.RuleID == tc.wantRule {
					if v.PatternID != tc.pattern {
						t.Fatalf("PatternID = %q, want %q", v.PatternID, tc.pattern)
					}
					if v.Location == "" || v.Line == 0 {
						t.Fatalf("missing location in %+v", v)
					}
					return
				}
			}
			for _, v := range se.Violations {
				t.Log(v.RuleID, v.String())
			}
			t.Fatalf("rule %s did not fire", tc.wantRule)
		})
	}
}

func TestValidateCreditNoteSkipsSchematron(t *testing.T) {
	t.Parallel()

	var inv invoice.Invoice
	if err := ubltr.Unmarshal(readFixture(t, "TemelFaturaOrnegi.xml"), &inv); err != nil {
		t.Fatal(err)
	}
	cn, err := ubltr.NewCreditNoteFromInvoice(&inv)
	if err != nil {
		t.Fatal(err)
	}
	data, err := ubltr.Marshal(cn)
	if err != nil {
		t.Fatal(err)
	}
	if err := ubltr.Validate(t.Context(), data); err != nil {
		t.Fatalf("Validate CreditNote: %v", err)
	}
}

func testParty(company bool) *builder.PartyBuilder {
	addr := builder.NewAddressBuilder().
		Street("Papatya Caddesi").
		Building("21").
		District("Beşiktaş").
		City("İstanbul").
		PostalZone("34100").
		Country("Türkiye").
		CountryCode("TR")
	if company {
		return builder.NewPartyBuilder().
			VKN("1288331521").
			Name("AAA Anonim Şirketi").
			TaxOffice("Büyük Mükellefler").
			AddressBuilder(addr)
	}
	return builder.NewPartyBuilder().
		TCKN("12345678901").
		Person("Ali", "Yılmaz").
		AddressBuilder(addr)
}

func validInvoice(t *testing.T) *invoice.Invoice {
	t.Helper()
	price, err := builder.MoneyIn(codelist.TRY, "10.00")
	if err != nil {
		t.Fatal(err)
	}
	qty, err := builder.QuantityIn(codelist.UnitC62, "2")
	if err != nil {
		t.Fatal(err)
	}
	inv, err := builder.NewInvoiceBuilder().
		Profile(codelist.ProfileTemelFatura).
		Type(codelist.InvoiceTypeSatis).
		ID("GIB2026000000001").
		IssueDate("2026-03-01").
		IssueTime("10:00:00").
		Currency(codelist.TRY).
		SupplierBuilder(testParty(true)).
		CustomerBuilder(testParty(false)).
		Line(builder.NewInvoiceLineBuilder().
			Name("Widget").
			Quantity(qty).
			Price(price).
			Tax(builder.NewTaxBuilder().KDV("20"))).
		Build()
	if err != nil {
		t.Fatalf("Build invoice: %v", err)
	}
	return inv
}

// validDespatch builds an e-İrsaliye with the builder and fills the shipment
// fields that the GİB despatch rules require (despatch date/time, delivery
// address, driver, and plate).
func validDespatch(t *testing.T) *despatchadvice.DespatchAdvice {
	t.Helper()
	qty, err := builder.QuantityIn(codelist.UnitC62, "2")
	if err != nil {
		t.Fatal(err)
	}
	d, err := builder.NewDespatchAdviceBuilder().
		Profile(codelist.ProfileTemelIrsaliye).
		Type(codelist.DespatchSEVK).
		ID("IRS2026000000001").
		IssueDate("2026-03-01").
		IssueTime("14:00:00").
		ShipmentID("1").
		SupplierBuilder(testParty(true)).
		CustomerBuilder(testParty(false)).
		Line(builder.NewDespatchLineBuilder().Name("Widget").Quantity(qty)).
		Build()
	if err != nil {
		t.Fatalf("Build despatch: %v", err)
	}
	postal := cbc.PostalZoneType{Value: "34100"}
	d.Shipment.ShipmentStage = []cac.ShipmentStageType{{
		TransportMeans: &cac.TransportMeansType{
			RoadTransport: &cac.RoadTransportType{
				LicensePlateID: cbc.LicensePlateIDType{SchemeID: "PLAKA", Value: "34ABC123"},
			},
		},
		DriverPerson: []cac.PersonType{{
			FirstName:     cbc.FirstNameType{Value: "Ayşe"},
			FamilyName:    cbc.FamilyNameType{Value: "Demir"},
			NationalityID: &cbc.NationalityIDType{Value: "12345678901"},
		}},
	}}
	d.Shipment.Delivery = &cac.DeliveryType{
		DeliveryAddress: &cac.AddressType{
			CitySubdivisionName: cbc.CitySubdivisionNameType{Value: "Kadıköy"},
			CityName:            cbc.CityNameType{Value: "İstanbul"},
			PostalZone:          &postal,
			Country:             cac.CountryType{Name: cbc.NameType{Value: "Türkiye"}},
		},
		Despatch: &cac.DespatchType{
			ActualDespatchDate: &cbc.ActualDespatchDateType{Value: "2026-03-01"},
			ActualDespatchTime: &cbc.ActualDespatchTimeType{Value: "14:00:00"},
		},
	}
	return d
}

func validApplicationResponse(t *testing.T) *applicationresponse.ApplicationResponse {
	t.Helper()
	ar, err := builder.NewApplicationResponseBuilder().
		Profile(codelist.ProfileTicariFatura).
		ID("APP2026000000001").
		IssueDate("2026-03-02").
		SenderBuilder(testParty(true)).
		ReceiverBuilder(testParty(false)).
		DocumentResponse("GIB2026000000001", "2026-03-01", codelist.ResponseKabul).
		Build()
	if err != nil {
		t.Fatalf("Build application response: %v", err)
	}
	ref := &ar.DocumentResponse[0].DocumentReference
	ref.DocumentTypeCode = &cbc.DocumentTypeCodeType{Value: "FATURA"}
	ref.DocumentType = &cbc.DocumentTypeType{Value: "FATURA"}
	return ar
}

func validReceipt(t *testing.T) any {
	t.Helper()
	qty, err := builder.QuantityIn(codelist.UnitC62, "2")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := builder.NewReceiptAdviceBuilder().
		Profile(codelist.ProfileTemelIrsaliye).
		Type(codelist.ReceiptSEVK).
		ID("KBL2026000000001").
		IssueDate("2026-03-02").
		DespatchReference("IRS2026000000001", "2026-03-01").
		SupplierBuilder(testParty(true)).
		CustomerBuilder(testParty(false)).
		Line(builder.NewReceiptLineBuilder().Name("Widget").Quantity(qty)).
		Build()
	if err != nil {
		t.Fatalf("Build receipt: %v", err)
	}
	return doc
}
