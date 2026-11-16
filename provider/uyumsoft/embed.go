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
	unsigned := !bytes.Contains(doc, []byte(nsXMLDSig))
	depth := 0
	var cuts [][2]int64 // top-level cac:Signature ranges to drop
	var sigStart int64 = -1
	for {
		off := dec.InputOffset()
		tok, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("uyumsoft: parse document: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if depth == 0 && unsigned && t.Name.Space == nsCAC && t.Name.Local == "Signature" {
				sigStart = off
			}
			depth++
		case xml.EndElement:
			if depth == 0 {
				out.Inner = bytes.TrimSpace(prepareUnsigned(doc[start:off], start, cuts, unsigned))
				return out, nil
			}
			depth--
			if depth == 0 && sigStart >= 0 {
				cuts = append(cuts, [2]int64{sigStart, dec.InputOffset()})
				sigStart = -1
			}
		}
	}
}

const (
	nsCAC      = "urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2"
	nsXMLDSig  = "http://www.w3.org/2000/09/xmldsig#"
	ublPlaceTR = `<UBLTR xmlns="urn:ubltr:extension"/>`
)

// prepareUnsigned readies an unsigned document for integrator signing. The
// integrator appends its own ds:Signature to ExtensionContent and its own
// cac:Signature, so the go-ubltr builder's stand-ins must not be there:
// left in, ExtensionContent gets two children (XSD error) and the document
// two cac:Signature elements (GİB Schematron error). Signed documents are
// returned unchanged so their digests stay valid.
func prepareUnsigned(inner []byte, base int64, cuts [][2]int64, unsigned bool) []byte {
	if !unsigned {
		return inner
	}
	out := make([]byte, 0, len(inner))
	pos := int64(0)
	for _, c := range cuts {
		from, to := c[0]-base, c[1]-base
		out = append(out, inner[pos:from]...)
		pos = to
	}
	out = append(out, inner[pos:]...)
	out = bytes.Replace(out, []byte(ublPlaceTR), nil, 1)
	return bytes.Replace(out, []byte(`<UBLTR xmlns="urn:ubltr:extension"></UBLTR>`), nil, 1)
}
