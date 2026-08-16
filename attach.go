package ubltr

import (
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/furkanmeclis/go-ubltr/codelist"
	"github.com/furkanmeclis/go-ubltr/schema/ubltr121/cac"
	"github.com/furkanmeclis/go-ubltr/schema/ubltr121/cbc"
	"github.com/furkanmeclis/go-ubltr/schema/ubltr121/creditnote"
	"github.com/furkanmeclis/go-ubltr/schema/ubltr121/invoice"
)

// DocRef is an AdditionalDocumentReference without embedded bytes.
type DocRef struct {
	ID          string
	IssueDate   string
	TypeCode    string
	Type        string
	Description string
}

// Attachment is a file stored on AdditionalDocumentReference as
// EmbeddedDocumentBinaryObject (base64). MIME must be a GİB MimeCodeList value.
// UBL-TR BinaryObjectType allows mimeCode only; Filename is kept as DocumentDescription
// when Description is empty.
type Attachment struct {
	ID          string
	IssueDate   string
	TypeCode    string
	Type        string
	Description string
	Filename    string
	MIME        string
	Data        []byte
}

// NewDocumentReference builds a cac.DocumentReferenceType from r.
func NewDocumentReference(r DocRef) (cac.DocumentReferenceType, error) {
	id := strings.TrimSpace(r.ID)
	date := strings.TrimSpace(r.IssueDate)
	if id == "" {
		return cac.DocumentReferenceType{}, fmt.Errorf("document reference: id is required")
	}
	if date == "" {
		return cac.DocumentReferenceType{}, fmt.Errorf("document reference: issue date is required")
	}
	ref := cac.DocumentReferenceType{
		ID:        cbc.IDType{Value: id},
		IssueDate: cbc.IssueDateType{Value: date},
	}
	if code := strings.TrimSpace(r.TypeCode); code != "" {
		ref.DocumentTypeCode = &cbc.DocumentTypeCodeType{Value: code}
	}
	if typ := strings.TrimSpace(r.Type); typ != "" {
		ref.DocumentType = &cbc.DocumentTypeType{Value: typ}
	}
	if desc := strings.TrimSpace(r.Description); desc != "" {
		ref.DocumentDescription = []cbc.DocumentDescriptionType{{Value: desc}}
	}
	return ref, nil
}

// NewAttachmentReference builds a document reference with embedded bytes.
func NewAttachmentReference(a Attachment) (cac.DocumentReferenceType, error) {
	mime := strings.TrimSpace(a.MIME)
	if mime == "" {
		return cac.DocumentReferenceType{}, fmt.Errorf("attachment: mime is required")
	}
	if !codelist.ValidMIME(mime) {
		return cac.DocumentReferenceType{}, fmt.Errorf("attachment: unknown mime %q", mime)
	}
	if len(a.Data) == 0 {
		return cac.DocumentReferenceType{}, fmt.Errorf("attachment: data is required")
	}
	desc := strings.TrimSpace(a.Description)
	if desc == "" {
		desc = strings.TrimSpace(a.Filename)
	}
	ref, err := NewDocumentReference(DocRef{
		ID:          a.ID,
		IssueDate:   a.IssueDate,
		TypeCode:    a.TypeCode,
		Type:        a.Type,
		Description: desc,
	})
	if err != nil {
		return cac.DocumentReferenceType{}, err
	}
	// UBL-TR BinaryObjectType allows mimeCode only (not encodingCode/filename).
	bin := cbc.EmbeddedDocumentBinaryObjectType{
		MimeCode: mime,
		Value:    base64.StdEncoding.EncodeToString(a.Data),
	}
	ref.Attachment = &cac.AttachmentType{
		EmbeddedDocumentBinaryObject: &bin,
	}
	return ref, nil
}

// AddAdditionalDocumentReference appends r to inv.
func AddAdditionalDocumentReference(inv *invoice.Invoice, r DocRef) error {
	if inv == nil {
		return fmt.Errorf("nil invoice")
	}
	if strings.TrimSpace(r.IssueDate) == "" {
		r.IssueDate = inv.IssueDate.Value
	}
	ref, err := NewDocumentReference(r)
	if err != nil {
		return err
	}
	inv.AdditionalDocumentReference = append(inv.AdditionalDocumentReference, ref)
	return nil
}

// AddAttachment appends an embedded AdditionalDocumentReference to inv.
func AddAttachment(inv *invoice.Invoice, a Attachment) error {
	if inv == nil {
		return fmt.Errorf("nil invoice")
	}
	if strings.TrimSpace(a.IssueDate) == "" {
		a.IssueDate = inv.IssueDate.Value
	}
	ref, err := NewAttachmentReference(a)
	if err != nil {
		return err
	}
	inv.AdditionalDocumentReference = append(inv.AdditionalDocumentReference, ref)
	return nil
}

// AddCreditNoteAttachment appends an embedded AdditionalDocumentReference to cn.
func AddCreditNoteAttachment(cn *creditnote.CreditNote, a Attachment) error {
	if cn == nil {
		return fmt.Errorf("nil credit note")
	}
	if strings.TrimSpace(a.IssueDate) == "" {
		a.IssueDate = cn.IssueDate.Value
	}
	ref, err := NewAttachmentReference(a)
	if err != nil {
		return err
	}
	cn.AdditionalDocumentReference = append(cn.AdditionalDocumentReference, ref)
	return nil
}
