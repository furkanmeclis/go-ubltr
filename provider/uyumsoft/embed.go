package uyumsoft

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
)

const nsUBLInvoice = "urn:oasis:names:specification:ubl:schema:xsd:Invoice-2"

// embeddedInvoice is InvoiceInfo/Invoice: an element in the service
// namespace (tns, elementFormDefault qualified) of type UBL InvoiceType,
// whose children are the cbc:/cac:/ext: elements of the UBL document.
// Both WSDLs type it this way (q2:InvoiceType / xs:any), and the
// muhasebeizleme guide warns that putting the UBL default namespace on
// <Invoice> hides the content from the service.
type embeddedInvoice struct {
	Attrs []xml.Attr `xml:",any,attr"`
	Inner []byte     `xml:",innerxml"`
}

// embedInvoice splits a UBL Invoice document into the namespace
// declarations of its root and the raw bytes of its children.
func embedInvoice(doc []byte) (*embeddedInvoice, error) {
	dec := xml.NewDecoder(bytes.NewReader(doc))
	var root *xml.StartElement
	var start int64
	for root == nil {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			return nil, errors.New("uyumsoft: document has no root element")
		}
		if err != nil {
			return nil, fmt.Errorf("uyumsoft: parse document: %w", err)
		}
		if se, ok := tok.(xml.StartElement); ok {
			se := se.Copy()
			root = &se
			start = dec.InputOffset()
		}
	}
	if root.Name.Local != "Invoice" || root.Name.Space != nsUBLInvoice {
		return nil, fmt.Errorf("uyumsoft: root element must be UBL Invoice, got {%s}%s", root.Name.Space, root.Name.Local)
	}
	out := &embeddedInvoice{}
	for _, a := range root.Attr {
		switch {
		case a.Name.Space == "xmlns":
			out.Attrs = append(out.Attrs, xml.Attr{Name: xml.Name{Local: "xmlns:" + a.Name.Local}, Value: a.Value})
		case a.Name.Space == "" && a.Name.Local == "xmlns":
			if a.Value != nsUBLInvoice {
				return nil, fmt.Errorf("uyumsoft: unexpected default namespace %q on Invoice", a.Value)
			}
		}
	}
	depth := 0
	for {
		off := dec.InputOffset()
		tok, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("uyumsoft: parse document: %w", err)
		}
		switch tok.(type) {
		case xml.StartElement:
			depth++
		case xml.EndElement:
			if depth == 0 {
				out.Inner = bytes.TrimSpace(doc[start:off])
				return out, nil
			}
			depth--
		}
	}
}
