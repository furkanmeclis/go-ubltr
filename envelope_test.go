package ubltr_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/go-ubltr"
	"github.com/furkanmeclis/go-ubltr/schema/envelope"
	"github.com/furkanmeclis/go-ubltr/schema/ubltr121/despatchadvice"
	"github.com/furkanmeclis/go-ubltr/schema/ubltr121/invoice"
)

func envelopeOptions() ubltr.EnvelopeOptions {
	return ubltr.EnvelopeOptions{
		InstanceIdentifier:  "2D5F4E6A-1B2C-4D3E-8F9A-0B1C2D3E4F5A",
		CreationDateAndTime: time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC),
		Sender: ubltr.EnvelopeParty{
			Alias:   "urn:mail:defaultgb@example.com",
			VKNTCKN: "1288331521",
			Name:    "AAA Anonim Şirketi",
		},
		Receiver: ubltr.EnvelopeParty{
			Alias:   "urn:mail:defaultpk@example.com",
			VKNTCKN: "12345678901",
		},
	}
}

func TestEnvelopeRoundTrip(t *testing.T) {
	t.Parallel()

	inv1 := validInvoice(t)
	inv2 := validInvoice(t)
	inv2.ID.Value = "GIB2026000000002"
	signed, err := ubltr.Marshal(validInvoice(t))
	if err != nil {
		t.Fatal(err)
	}

	env, err := ubltr.NewEnvelope(envelopeOptions(), inv1, inv2, signed)
	if err != nil {
		t.Fatalf("NewEnvelope: %v", err)
	}
	hdr := env.StandardBusinessDocumentHeader
	if got := hdr.DocumentIdentification.Type; got != ubltr.EnvelopeTypeSender {
		t.Fatalf("Type = %q", got)
	}
	if got := hdr.Sender[0].Identifier.Value; got != "urn:mail:defaultgb@example.com" {
		t.Fatalf("sender alias = %q", got)
	}
	if n := len(env.Package.Elements); n != 1 {
		t.Fatalf("Elements = %d, want 1", n)
	}
	if got := env.Package.Elements[0].ElementCount; got != "3" {
		t.Fatalf("ElementCount = %q", got)
	}

	data, err := ubltr.Marshal(env)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	kind, err := ubltr.Detect(data)
	if err != nil || kind != ubltr.KindEnvelope {
		t.Fatalf("Detect = %q, %v", kind, err)
	}
	if err := ubltr.Validate(t.Context(), data); err != nil {
		for _, v := range ubltr.Violations(err) {
			t.Log(v.String())
		}
		t.Fatalf("Validate envelope: %v\n%s", err, data)
	}

	sbd, docs, err := ubltr.UnmarshalEnvelope(data)
	if err != nil {
		t.Fatalf("UnmarshalEnvelope: %v", err)
	}
	if sbd.StandardBusinessDocumentHeader.DocumentIdentification.InstanceIdentifier != "2D5F4E6A-1B2C-4D3E-8F9A-0B1C2D3E4F5A" {
		t.Fatalf("instance id = %q", sbd.StandardBusinessDocumentHeader.DocumentIdentification.InstanceIdentifier)
	}
	if got := sbd.Package.Elements[0].ElementType; got != ubltr.ElementTypeInvoice {
		t.Fatalf("ElementType = %q", got)
	}
	if len(docs) != 3 {
		t.Fatalf("documents = %d, want 3", len(docs))
	}
	wantIDs := []string{"GIB2026000000001", "GIB2026000000002", "GIB2026000000001"}
	for i, d := range docs {
		if d.Kind != ubltr.KindInvoice || d.ElementType != ubltr.ElementTypeInvoice {
			t.Fatalf("doc %d kind = %q / %q", i, d.Kind, d.ElementType)
		}
		inv, ok := d.Document.(*invoice.Invoice)
		if !ok {
			t.Fatalf("doc %d is %T", i, d.Document)
		}
		if inv.ID.Value != wantIDs[i] {
			t.Fatalf("doc %d ID = %q, want %q", i, inv.ID.Value, wantIDs[i])
		}
		if err := ubltr.Validate(t.Context(), d.XML); err != nil {
			t.Fatalf("doc %d standalone Validate: %v", i, err)
		}
	}
}

func TestEnvelopePrefixedDocuments(t *testing.T) {
	t.Parallel()

	// Prefixes declared on the envelope root, not on the invoice.
	data := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<sh:StandardBusinessDocument xmlns:sh="http://www.unece.org/cefact/namespaces/StandardBusinessDocumentHeader" xmlns:ef="http://www.efatura.gov.tr/package-namespace" xmlns:inv="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2" xmlns:cbc="urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2">
  <ef:Package>
    <Elements>
      <ElementType>INVOICE</ElementType>
      <ElementCount>1</ElementCount>
      <ElementList><inv:Invoice><cbc:ID>ABC2026000000001</cbc:ID></inv:Invoice></ElementList>
    </Elements>
  </ef:Package>
</sh:StandardBusinessDocument>
`)
	docs, err := ubltr.EnvelopeDocuments(data)
	if err != nil {
		t.Fatalf("EnvelopeDocuments: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("documents = %d", len(docs))
	}
	inv := docs[0].Document.(*invoice.Invoice)
	if inv.ID.Value != "ABC2026000000001" {
		t.Fatalf("ID = %q", inv.ID.Value)
	}
	if kind, err := ubltr.Detect(docs[0].XML); err != nil || kind != ubltr.KindInvoice {
		t.Fatalf("standalone XML Detect = %q, %v\n%s", kind, err, docs[0].XML)
	}
}

func TestEnvelopeMixedTypesAndPostbox(t *testing.T) {
	t.Parallel()

	env, err := ubltr.NewEnvelope(envelopeOptions(), validApplicationResponse(t))
	if err != nil {
		t.Fatal(err)
	}
	if got := env.StandardBusinessDocumentHeader.DocumentIdentification.Type; got != ubltr.EnvelopeTypePostbox {
		t.Fatalf("Type = %q, want POSTBOXENVELOPE", got)
	}

	env, err = ubltr.NewEnvelope(envelopeOptions(), validDespatch(t), validInvoice(t), validDespatch(t))
	if err != nil {
		t.Fatal(err)
	}
	var types []string
	for _, e := range env.Package.Elements {
		types = append(types, e.ElementType+"="+e.ElementCount)
	}
	if got := strings.Join(types, ","); got != "DESPATCHADVICE=2,INVOICE=1" {
		t.Fatalf("Elements = %s", got)
	}
	data, err := ubltr.Marshal(env)
	if err != nil {
		t.Fatal(err)
	}
	if err := ubltr.ValidateXSD(t.Context(), data); err != nil {
		t.Fatalf("ValidateXSD: %v", err)
	}
	docs, err := ubltr.EnvelopeDocuments(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 3 {
		t.Fatalf("documents = %d", len(docs))
	}
	if _, ok := docs[0].Document.(*despatchadvice.DespatchAdvice); !ok || docs[0].ElementType != ubltr.ElementTypeDespatchAdvice {
		t.Fatalf("doc 0 = %T %s", docs[0].Document, docs[0].ElementType)
	}
	if docs[2].ElementType != ubltr.ElementTypeInvoice {
		t.Fatalf("doc 2 element type = %s", docs[2].ElementType)
	}
}

func TestEnvelopeSchematronSenderMismatch(t *testing.T) {
	t.Parallel()

	opts := envelopeOptions()
	opts.Sender.VKNTCKN = "9999999999"
	env, err := ubltr.NewEnvelope(opts, validInvoice(t))
	if err != nil {
		t.Fatal(err)
	}
	data, err := ubltr.Marshal(env)
	if err != nil {
		t.Fatal(err)
	}
	err = ubltr.Validate(t.Context(), data)
	var se *ubltr.SchematronValidationError
	if !errors.As(err, &se) {
		t.Fatalf("error %T %v, want *SchematronValidationError", err, err)
	}
	for _, v := range se.Violations {
		if v.RuleID == "DocumentSenderCheck" {
			if !strings.HasPrefix(v.Location, "/sh:StandardBusinessDocument/ef:Package/Elements/ElementList/inv:Invoice/") {
				t.Fatalf("Location = %q", v.Location)
			}
			return
		}
	}
	t.Fatalf("DocumentSenderCheck did not fire: %+v", se.Violations)
}

func TestEnvelopeXSDRejectsBadCount(t *testing.T) {
	t.Parallel()

	env, err := ubltr.NewEnvelope(envelopeOptions(), validInvoice(t))
	if err != nil {
		t.Fatal(err)
	}
	env.Package.Elements[0].ElementCount = "one"
	data, err := ubltr.Marshal(env)
	if err != nil {
		t.Fatal(err)
	}
	var xe *ubltr.XSDValidationError
	if err := ubltr.ValidateXSD(t.Context(), data); !errors.As(err, &xe) {
		t.Fatalf("error %T %v, want *XSDValidationError", err, err)
	}
	if xe.Violations[0].Location != "/sh:StandardBusinessDocument/ef:Package/Elements/ElementCount" {
		t.Fatalf("Location = %q", xe.Violations[0].Location)
	}
}

func TestNewEnvelopeErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		opts ubltr.EnvelopeOptions
		docs []any
	}{
		{name: "no documents", opts: envelopeOptions()},
		{name: "missing ids", opts: ubltr.EnvelopeOptions{}, docs: []any{ubltr.NewInvoice()}},
		{name: "unsupported type", opts: envelopeOptions(), docs: []any{ubltr.NewEArchiveReport()}},
		{name: "unknown bytes", opts: envelopeOptions(), docs: []any{[]byte(`<Foo/>`)}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, err := ubltr.NewEnvelope(tc.opts, tc.docs...); !errors.Is(err, ubltr.ErrEnvelope) {
				t.Fatalf("error = %v, want ErrEnvelope", err)
			}
		})
	}
}

func TestEnvelopeDocumentsRejectsNonEnvelope(t *testing.T) {
	t.Parallel()

	if _, err := ubltr.EnvelopeDocuments(readFixture(t, "TemelFaturaOrnegi.xml")); !errors.Is(err, ubltr.ErrEnvelope) {
		t.Fatalf("error = %v, want ErrEnvelope", err)
	}
	var sbd envelope.StandardBusinessDocument
	if err := ubltr.Unmarshal([]byte(`<sh:StandardBusinessDocument xmlns:sh="`+envelope.NamespaceSBDH+`"/>`), &sbd); err != nil {
		t.Fatal(err)
	}
}
