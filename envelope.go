package ubltr

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/go-ubltr/internal/validation"
	"github.com/furkanmeclis/go-ubltr/schema/envelope"
	"github.com/furkanmeclis/go-ubltr/schema/ubltr121/applicationresponse"
	"github.com/furkanmeclis/go-ubltr/schema/ubltr121/creditnote"
	"github.com/furkanmeclis/go-ubltr/schema/ubltr121/despatchadvice"
	"github.com/furkanmeclis/go-ubltr/schema/ubltr121/invoice"
	"github.com/furkanmeclis/go-ubltr/schema/ubltr121/receiptadvice"
)

// GİB envelope types (sh:DocumentIdentification/sh:Type).
const (
	EnvelopeTypeSender  = "SENDERENVELOPE"
	EnvelopeTypePostbox = "POSTBOXENVELOPE"
	EnvelopeTypeSystem  = "SYSTEMENVELOPE"
	EnvelopeTypeUser    = "USERENVELOPE"
)

// GİB package element types (ef:Package/Elements/ElementType).
const (
	ElementTypeInvoice             = "INVOICE"
	ElementTypeCreditNote          = "CREDITNOTE"
	ElementTypeApplicationResponse = "APPLICATIONRESPONSE"
	ElementTypeDespatchAdvice      = "DESPATCHADVICE"
	ElementTypeReceiptAdvice       = "RECEIPTADVICE"
)

// EnvelopeSchemaLocation is the xsi:schemaLocation GİB Schematron expects on
// sh:StandardBusinessDocument.
const EnvelopeSchemaLocation = envelope.NamespaceSBDH + " PackageProxy_1_2.xsd"

// ErrEnvelope is wrapped by [NewEnvelope] and [EnvelopeDocuments] errors.
var ErrEnvelope = errors.New("ubltr envelope")

// EnvelopeParty is the sender or receiver of a GİB envelope.
type EnvelopeParty struct {
	// Alias is the GİB etiket written to sh:Identifier, for example
	// urn:mail:defaultgb@example.com (sender) or urn:mail:defaultpk@example.com
	// (receiver).
	Alias string
	// VKNTCKN is the tax or national ID written as the VKN_TCKN contact.
	VKNTCKN string
	// Name is optional; it is written as the UNVAN contact.
	Name string
}

// EnvelopeOptions configures [NewEnvelope].
type EnvelopeOptions struct {
	// Type is the envelope type. When empty it is SENDERENVELOPE for
	// Invoice, CreditNote, and DespatchAdvice and POSTBOXENVELOPE for
	// ApplicationResponse and ReceiptAdvice.
	Type string
	// InstanceIdentifier is the envelope UUID. A random UUID is used when empty.
	InstanceIdentifier string
	// CreationDateAndTime defaults to the current local time.
	CreationDateAndTime time.Time
	// HeaderVersion defaults to "1.0".
	HeaderVersion string
	Sender        EnvelopeParty
	Receiver      EnvelopeParty
}

// EnvelopeDocument is one document carried in an envelope's ef:Package.
type EnvelopeDocument struct {
	// ElementType is the GİB ElementType of the Elements group (INVOICE, …).
	ElementType string
	// Kind is the detected document kind.
	Kind Kind
	// XML is the document as a standalone XML file. It is the original bytes
	// from the envelope when they parse on their own (signatures stay
	// intact); otherwise the typed Document is marshaled again.
	XML []byte
	// Document is the decoded document: *invoice.Invoice,
	// *creditnote.CreditNote, *applicationresponse.ApplicationResponse,
	// *despatchadvice.DespatchAdvice, or *receiptadvice.ReceiptAdvice.
	Document any
}

// NewEnvelope builds a GİB envelope (sh:StandardBusinessDocument) whose
// ef:Package holds docs, grouped into one Elements entry per element type
// in first-seen order. Each doc is a pointer to a generated UBL-TR document
// (*invoice.Invoice, *creditnote.CreditNote, *despatchadvice.DespatchAdvice,
// *applicationresponse.ApplicationResponse, *receiptadvice.ReceiptAdvice)
// or []byte of an already serialized one (for example a signed invoice).
//
// Marshal the result with [Marshal]; validate it with [Validate].
func NewEnvelope(opts EnvelopeOptions, docs ...any) (*envelope.StandardBusinessDocument, error) {
	if len(docs) == 0 {
		return nil, fmt.Errorf("%w: no documents", ErrEnvelope)
	}
	if strings.TrimSpace(opts.Sender.VKNTCKN) == "" || strings.TrimSpace(opts.Receiver.VKNTCKN) == "" {
		return nil, fmt.Errorf("%w: sender and receiver VKNTCKN are required", ErrEnvelope)
	}

	var order []string
	groups := map[string][]string{}
	for i, d := range docs {
		elementType, body, err := envelopeBody(d)
		if err != nil {
			return nil, fmt.Errorf("%w: document %d: %w", ErrEnvelope, i, err)
		}
		if _, ok := groups[elementType]; !ok {
			order = append(order, elementType)
		}
		groups[elementType] = append(groups[elementType], body)
	}

	typ := opts.Type
	if typ == "" {
		typ = EnvelopeTypeSender
		if order[0] == ElementTypeApplicationResponse || order[0] == ElementTypeReceiptAdvice {
			typ = EnvelopeTypePostbox
		}
	}
	id := opts.InstanceIdentifier
	if id == "" {
		u, err := newDocumentUUID()
		if err != nil {
			return nil, err
		}
		id = u
	}
	created := opts.CreationDateAndTime
	if created.IsZero() {
		created = time.Now()
	}
	version := opts.HeaderVersion
	if version == "" {
		version = "1.0"
	}

	pkg := &envelope.Package{}
	for _, et := range order {
		bodies := groups[et]
		pkg.Elements = append(pkg.Elements, envelope.Elements{
			ElementType:  et,
			ElementCount: strconv.Itoa(len(bodies)),
			ElementList:  envelope.ElementList{InnerXML: "\n" + strings.Join(bodies, "\n") + "\n"},
		})
	}

	return &envelope.StandardBusinessDocument{
		SchemaLocation: EnvelopeSchemaLocation,
		StandardBusinessDocumentHeader: &envelope.StandardBusinessDocumentHeader{
			HeaderVersion: version,
			Sender:        []envelope.Partner{envelopePartner(opts.Sender)},
			Receiver:      []envelope.Partner{envelopePartner(opts.Receiver)},
			DocumentIdentification: envelope.DocumentIdentification{
				Standard:            "UBL-TR",
				TypeVersion:         "1.2",
				InstanceIdentifier:  id,
				Type:                typ,
				CreationDateAndTime: created.Format("2006-01-02T15:04:05"),
			},
		},
		Package: pkg,
	}, nil
}

func envelopePartner(p EnvelopeParty) envelope.Partner {
	out := envelope.Partner{
		Identifier: envelope.PartnerIdentification{Value: p.Alias},
		ContactInformation: []envelope.ContactInformation{{
			Contact:               p.VKNTCKN,
			ContactTypeIdentifier: "VKN_TCKN",
		}},
	}
	if p.Name != "" {
		out.ContactInformation = append(out.ContactInformation, envelope.ContactInformation{
			Contact:               p.Name,
			ContactTypeIdentifier: "UNVAN",
		})
	}
	return out
}

func envelopeBody(doc any) (elementType, body string, err error) {
	switch d := doc.(type) {
	case []byte:
		kind, err := Detect(d)
		if err != nil {
			return "", "", err
		}
		et, ok := elementTypeOf(kind)
		if !ok {
			return "", "", fmt.Errorf("%s cannot be packaged", kind)
		}
		return et, string(bytes.TrimSpace(stripXMLDecl(d))), nil
	case *invoice.Invoice:
		elementType = ElementTypeInvoice
	case *creditnote.CreditNote:
		elementType = ElementTypeCreditNote
	case *applicationresponse.ApplicationResponse:
		elementType = ElementTypeApplicationResponse
	case *despatchadvice.DespatchAdvice:
		elementType = ElementTypeDespatchAdvice
	case *receiptadvice.ReceiptAdvice:
		elementType = ElementTypeReceiptAdvice
	default:
		return "", "", fmt.Errorf("unsupported document type %T", doc)
	}
	out, err := xml.Marshal(doc)
	if err != nil {
		return "", "", fmt.Errorf("marshal: %w", err)
	}
	return elementType, string(out), nil
}

func stripXMLDecl(b []byte) []byte {
	t := bytes.TrimPrefix(b, []byte("\xef\xbb\xbf"))
	t = bytes.TrimLeft(t, " \t\r\n")
	if bytes.HasPrefix(t, []byte("<?xml")) {
		if i := bytes.Index(t, []byte("?>")); i >= 0 {
			return t[i+2:]
		}
	}
	return t
}

func elementTypeOf(k Kind) (string, bool) {
	switch k {
	case KindInvoice:
		return ElementTypeInvoice, true
	case KindCreditNote:
		return ElementTypeCreditNote, true
	case KindApplicationResponse:
		return ElementTypeApplicationResponse, true
	case KindDespatchAdvice:
		return ElementTypeDespatchAdvice, true
	case KindReceiptAdvice:
		return ElementTypeReceiptAdvice, true
	default:
		return "", false
	}
}

func newEnvelopeDocument(k Kind) (any, bool) {
	switch k {
	case KindInvoice:
		return NewInvoice(), true
	case KindCreditNote:
		return NewCreditNote(), true
	case KindApplicationResponse:
		return NewApplicationResponse(), true
	case KindDespatchAdvice:
		return NewDespatchAdvice(), true
	case KindReceiptAdvice:
		return NewReceiptAdvice(), true
	default:
		return nil, false
	}
}

// UnmarshalEnvelope decodes a GİB envelope and the documents in its
// ef:Package. See [EnvelopeDocuments].
func UnmarshalEnvelope(data []byte) (*envelope.StandardBusinessDocument, []EnvelopeDocument, error) {
	var sbd envelope.StandardBusinessDocument
	if err := Unmarshal(data, &sbd); err != nil {
		return nil, nil, err
	}
	docs, err := EnvelopeDocuments(data)
	if err != nil {
		return nil, nil, err
	}
	return &sbd, docs, nil
}

// EnvelopeDocuments returns the UBL-TR documents inside the ef:Package of
// the envelope in data, in document order. Namespaces declared on envelope
// ancestors are honoured when decoding. Elements whose root is not a
// supported UBL-TR document (for example HR-XML user accounts) return an
// error wrapping [ErrUnknownDocument].
func EnvelopeDocuments(data []byte) ([]EnvelopeDocument, error) {
	kind, err := Detect(data)
	if err != nil {
		return nil, err
	}
	if kind != KindEnvelope {
		return nil, fmt.Errorf("%w: root is %s, not an envelope", ErrEnvelope, kind)
	}

	dec := xml.NewDecoder(bytes.NewReader(data))
	var (
		out         []EnvelopeDocument
		stack       []xml.Name
		elementType string
		inType      bool
	)
	for {
		start := dec.InputOffset()
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrEnvelope, err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if len(stack) > 0 && stack[len(stack)-1].Local == "ElementList" && inPackage(stack) {
				doc, err := decodeEnvelopeChild(dec, data, start, t, elementType)
				if err != nil {
					return nil, err
				}
				out = append(out, doc)
				continue
			}
			stack = append(stack, t.Name)
			inType = t.Name.Local == "ElementType" && inPackage(stack[:len(stack)-1])
			if t.Name.Local == "Elements" {
				elementType = ""
			}
		case xml.CharData:
			if inType {
				elementType += string(t)
			}
		case xml.EndElement:
			if inType {
				elementType = strings.TrimSpace(elementType)
				inType = false
			}
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		}
	}
}

func inPackage(stack []xml.Name) bool {
	for _, n := range stack {
		if n.Space == envelope.NamespacePackage && n.Local == "Package" {
			return true
		}
	}
	return false
}

func decodeEnvelopeChild(dec *xml.Decoder, data []byte, start int64, se xml.StartElement, elementType string) (EnvelopeDocument, error) {
	vk, err := validation.KindOf(se.Name)
	if err != nil {
		return EnvelopeDocument{}, err
	}
	kind := Kind(vk)
	doc, ok := newEnvelopeDocument(kind)
	if !ok {
		return EnvelopeDocument{}, fmt.Errorf("%w: %s inside ef:Package", ErrUnknownDocument, kind)
	}
	if err := dec.DecodeElement(doc, &se); err != nil {
		return EnvelopeDocument{}, fmt.Errorf("%w: decode %s: %w", ErrEnvelope, kind, err)
	}
	raw := bytes.TrimSpace(data[start:dec.InputOffset()])
	standalone := make([]byte, 0, len(xml.Header)+len(raw)+1)
	standalone = append(standalone, xml.Header...)
	standalone = append(standalone, raw...)
	standalone = append(standalone, '\n')
	if validation.WellFormed(context.Background(), standalone) != nil {
		if standalone, err = Marshal(doc); err != nil {
			return EnvelopeDocument{}, err
		}
	}
	if elementType == "" {
		elementType, _ = elementTypeOf(kind)
	}
	return EnvelopeDocument{
		ElementType: elementType,
		Kind:        kind,
		XML:         standalone,
		Document:    doc,
	}, nil
}
