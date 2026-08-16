package ubltr_test

import (
	"encoding/base64"
	"testing"

	"github.com/furkanmeclis/go-ubltr"
	"github.com/furkanmeclis/go-ubltr/schema/ubltr121/invoice"
)

func TestAddAttachment(t *testing.T) {
	t.Parallel()

	var inv invoice.Invoice
	if err := ubltr.Unmarshal(readFixture(t, "TemelFaturaOrnegi.xml"), &inv); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(inv.UBLExtensions.UBLExtension) > 0 {
		inv.UBLExtensions.UBLExtension[0].ExtensionContent.InnerXML = `<UBLTR xmlns="urn:ubltr:extension"/>`
	}

	payload := []byte("%PDF-1.4 test")
	err := ubltr.AddAttachment(&inv, ubltr.Attachment{
		ID:       "ATT-1",
		Filename: "note.pdf",
		MIME:     "application/pdf",
		Data:     payload,
	})
	if err != nil {
		t.Fatalf("AddAttachment: %v", err)
	}
	if len(inv.AdditionalDocumentReference) == 0 {
		t.Fatal("no additional document reference")
	}
	ref := inv.AdditionalDocumentReference[len(inv.AdditionalDocumentReference)-1]
	if ref.ID.Value != "ATT-1" {
		t.Fatalf("ID = %q", ref.ID.Value)
	}
	if ref.IssueDate.Value != inv.IssueDate.Value {
		t.Fatalf("IssueDate = %q, want invoice date", ref.IssueDate.Value)
	}
	bin := ref.Attachment.EmbeddedDocumentBinaryObject
	if bin.MimeCode != "application/pdf" {
		t.Fatalf("MimeCode = %q", bin.MimeCode)
	}
	got, err := base64.StdEncoding.DecodeString(bin.Value)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if string(got) != string(payload) {
		t.Fatalf("payload = %q", got)
	}

	data, err := ubltr.Marshal(&inv)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if err := ubltr.ValidateXSD(t.Context(), data); err != nil {
		t.Fatalf("ValidateXSD: %v", err)
	}

	var again invoice.Invoice
	if err := ubltr.Unmarshal(data, &again); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	againBin := again.AdditionalDocumentReference[len(again.AdditionalDocumentReference)-1].Attachment.EmbeddedDocumentBinaryObject
	round, err := base64.StdEncoding.DecodeString(againBin.Value)
	if err != nil {
		t.Fatalf("round-trip decode: %v", err)
	}
	if string(round) != string(payload) {
		t.Fatalf("round-trip payload = %q", round)
	}
}

func TestAddAdditionalDocumentReference(t *testing.T) {
	t.Parallel()

	var inv invoice.Invoice
	if err := ubltr.Unmarshal(readFixture(t, "TemelFaturaOrnegi.xml"), &inv); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	n := len(inv.AdditionalDocumentReference)
	err := ubltr.AddAdditionalDocumentReference(&inv, ubltr.DocRef{
		ID:   "ORD-1",
		Type: "ORDER",
	})
	if err != nil {
		t.Fatalf("AddAdditionalDocumentReference: %v", err)
	}
	if len(inv.AdditionalDocumentReference) != n+1 {
		t.Fatalf("refs = %d, want %d", len(inv.AdditionalDocumentReference), n+1)
	}
}

func TestAddAttachmentRejectsUnknownMIME(t *testing.T) {
	t.Parallel()

	inv := ubltr.NewInvoice()
	inv.IssueDate.Value = "2026-03-01"
	err := ubltr.AddAttachment(inv, ubltr.Attachment{
		ID:   "ATT-1",
		MIME: "application/x-unknown",
		Data: []byte("x"),
	})
	if err == nil {
		t.Fatal("expected error")
	}
}
