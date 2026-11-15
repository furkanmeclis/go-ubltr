package isnet

import (
	"encoding/xml"
	"strings"
	"time"
)

// Wire types for the İşNet WCF services. Element names, namespaces and
// order follow the WSDL kept under testdata/wsdl. WCF reads data-contract
// members in alphabetical order and ignores members out of order, so every
// struct below lists its fields alphabetically; empty fields are omitted.

const (
	actionInvoice     = "http://tempuri.org/IInvoiceService/"
	actionAddressBook = "http://tempuri.org/IAddressBookService/"
)

// Enum values as İşNet serialises them (by name).
const (
	dirIncoming = "Incoming"
	dirOutgoing = "Outgoing"

	docEInvoice = "EInvoice"
	docEArchive = "EArchiveInvoice"
)

// wireTime is the İşNet date-time format: no zone, Turkey local time.
const wireTime = "2006-01-02T15:04:05"

// turkey is Türkiye time (UTC+3, no daylight saving since 2016).
var turkey = time.FixedZone("TRT", 3*60*60)

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.In(turkey).Format(wireTime)
}

// parseTime reads xs:dateTime values with or without fraction and zone.
// Values without a zone are Türkiye time.
func parseTime(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t
	}
	for _, layout := range []string{"2006-01-02T15:04:05.9999999", wireTime, "2006-01-02"} {
		if t, err := time.ParseInLocation(layout, s, turkey); err == nil {
			return t
		}
	}
	return time.Time{}
}

type stringList struct {
	Items []string `xml:"http://schemas.microsoft.com/2003/10/Serialization/Arrays string"`
}

type pagingRequest struct {
	PageNumber     int `xml:"http://schemas.datacontract.org/2004/07/EInvoice.Service.Model PageNumber"`
	RecordsPerPage int `xml:"http://schemas.datacontract.org/2004/07/EInvoice.Service.Model RecordsPerPage"`
}

// resultSet is InvoiceResultSet. All members are always sent.
type resultSet struct {
	IsAdditionalTaxIncluded bool `xml:"http://schemas.datacontract.org/2004/07/EInvoice.Service.Model IsAdditionalTaxIncluded"`
	IsArchiveIncluded       bool `xml:"http://schemas.datacontract.org/2004/07/EInvoice.Service.Model IsArchiveIncluded"`
	IsAttachmentIncluded    bool `xml:"http://schemas.datacontract.org/2004/07/EInvoice.Service.Model IsAttachmentIncluded"`
	IsExternalURLIncluded   bool `xml:"http://schemas.datacontract.org/2004/07/EInvoice.Service.Model IsExternalUrlIncluded"`
	IsHTMLIncluded          bool `xml:"http://schemas.datacontract.org/2004/07/EInvoice.Service.Model IsHtmlIncluded"`
	IsInvoiceDetailIncluded bool `xml:"http://schemas.datacontract.org/2004/07/EInvoice.Service.Model IsInvoiceDetailIncluded"`
	IsPDFIncluded           bool `xml:"http://schemas.datacontract.org/2004/07/EInvoice.Service.Model IsPDFIncluded"`
	IsXMLIncluded           bool `xml:"http://schemas.datacontract.org/2004/07/EInvoice.Service.Model IsXMLIncluded"`
}

// newResultSet always includes archived documents (IsArchiveIncluded),
// so invoices the company archived in the İşNet portal are not missed.
func newResultSet(f content) resultSet {
	return resultSet{IsArchiveIncluded: true, IsXMLIncluded: f.xml, IsPDFIncluded: f.pdf, IsHTMLIncluded: f.html}
}

// content selects the document bodies a search returns.
type content struct{ xml, pdf, html bool }

// searchAllInvoiceRequest is SearchAllInvoiceRequest (members used here).
type searchAllInvoiceRequest struct {
	CompanyTaxCode                 string        `xml:"http://schemas.datacontract.org/2004/07/EInvoice.Service.Model CompanyTaxCode"`
	Ettn                           string        `xml:"http://schemas.datacontract.org/2004/07/EInvoice.Service.Model Ettn,omitempty"`
	IncludeCompanyVendorNumberList *stringList   `xml:"http://schemas.datacontract.org/2004/07/EInvoice.Service.Model IncludeCompanyVendorNumberList,omitempty"`
	IncludeMainCompany             bool          `xml:"http://schemas.datacontract.org/2004/07/EInvoice.Service.Model IncludeMainCompany"`
	InvoiceDirection               string        `xml:"http://schemas.datacontract.org/2004/07/EInvoice.Service.Model InvoiceDirection"`
	MaxInvoiceCreationDate         string        `xml:"http://schemas.datacontract.org/2004/07/EInvoice.Service.Model MaxInvoiceCreationDate,omitempty"`
	MinInvoiceCreationDate         string        `xml:"http://schemas.datacontract.org/2004/07/EInvoice.Service.Model MinInvoiceCreationDate,omitempty"`
	PagingRequest                  pagingRequest `xml:"http://schemas.datacontract.org/2004/07/EInvoice.Service.Model PagingRequest"`
	ResultSet                      resultSet     `xml:"http://schemas.datacontract.org/2004/07/EInvoice.Service.Model ResultSet"`
}

type searchAllInvoice struct {
	XMLName xml.Name                `xml:"http://tempuri.org/ SearchAllInvoice"`
	Request searchAllInvoiceRequest `xml:"http://tempuri.org/ request"`
}

// searchArchiveInvoiceRequest is SearchArchiveInvoiceRequest (members used
// here). The WSDL names the result-set member ResultSet, as in İşNet's
// request samples.
type searchArchiveInvoiceRequest struct {
	CompanyTaxCode         string        `xml:"http://schemas.datacontract.org/2004/07/EInvoice.Service.Model CompanyTaxCode"`
	Ettn                   string        `xml:"http://schemas.datacontract.org/2004/07/EInvoice.Service.Model Ettn,omitempty"`
	IsCancelled            *bool         `xml:"http://schemas.datacontract.org/2004/07/EInvoice.Service.Model IsCancelled,omitempty"`
	MaxInvoiceCreationDate string        `xml:"http://schemas.datacontract.org/2004/07/EInvoice.Service.Model MaxInvoiceCreationDate,omitempty"`
	MinInvoiceCreationDate string        `xml:"http://schemas.datacontract.org/2004/07/EInvoice.Service.Model MinInvoiceCreationDate,omitempty"`
	PagingRequest          pagingRequest `xml:"http://schemas.datacontract.org/2004/07/EInvoice.Service.Model PagingRequest"`
	ResultSet              resultSet     `xml:"http://schemas.datacontract.org/2004/07/EInvoice.Service.Model ResultSet"`
}

type searchArchiveInvoice struct {
	XMLName xml.Name                    `xml:"http://tempuri.org/ SearchArchiveInvoice"`
	Request searchArchiveInvoiceRequest `xml:"http://tempuri.org/ request"`
}

type documentViewerLinkRequest struct {
	CompanyTaxCode      string `xml:"http://schemas.datacontract.org/2004/07/EInvoice.Service.Model CompanyTaxCode"`
	Ettn                string `xml:"http://schemas.datacontract.org/2004/07/EInvoice.Service.Model Ettn"`
	InvoiceDirection    string `xml:"http://schemas.datacontract.org/2004/07/EInvoice.Service.Model InvoiceDirection"`
	InvoiceDocumentType string `xml:"http://schemas.datacontract.org/2004/07/EInvoice.Service.Model InvoiceDocumentType"`
}

type getDocumentViewerLink struct {
	XMLName xml.Name                  `xml:"http://tempuri.org/ GetDocumentViewerLink"`
	Request documentViewerLinkRequest `xml:"http://tempuri.org/ request"`
}

type getVendorRequest struct {
	CompanyTaxCode string `xml:"http://schemas.datacontract.org/2004/07/EInvoice.Service.Model CompanyTaxCode"`
}

type getCompanyVendor struct {
	XMLName xml.Name         `xml:"http://tempuri.org/ GetCompanyVendor"`
	Request getVendorRequest `xml:"http://tempuri.org/ request"`
}

type getTaxPayerRequest struct {
	TaxPayerTaxCode string `xml:"http://schemas.datacontract.org/2004/07/EInvoice.Service.Model TaxPayerTaxCode"`
}

type getTaxPayer struct {
	XMLName xml.Name           `xml:"http://tempuri.org/ GetTaxPayer"`
	Request getTaxPayerRequest `xml:"http://tempuri.org/ request"`
}

// node is a generic element tree. Responses are read through it with
// case-insensitive local names, because the interface documents and the
// community integrations spell some members differently (Ettn / ETTN,
// Taxpayertaxcode / TaxPayerTaxCode).
type node struct {
	name     string
	nil      bool
	text     string
	children []*node
}

// UnmarshalXML implements xml.Unmarshaler.
func (n *node) UnmarshalXML(dec *xml.Decoder, start xml.StartElement) error {
	n.name = start.Name.Local
	for _, a := range start.Attr {
		if a.Name.Local == "nil" && a.Value == "true" {
			n.nil = true
		}
	}
	var text strings.Builder
	for {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			c := &node{}
			if err := c.UnmarshalXML(dec, t); err != nil {
				return err
			}
			n.children = append(n.children, c)
		case xml.CharData:
			if len(n.children) == 0 {
				text.Write(t)
			}
		case xml.EndElement:
			if len(n.children) == 0 && !n.nil {
				n.text = strings.TrimSpace(text.String())
			}
			return nil
		}
	}
}

// child returns the first child with the given local name, ignoring case.
func (n *node) child(name string) *node {
	if n == nil {
		return nil
	}
	for _, c := range n.children {
		if strings.EqualFold(c.name, name) {
			return c
		}
	}
	return nil
}

// path follows child names.
func (n *node) path(names ...string) *node {
	for _, name := range names {
		n = n.child(name)
	}
	return n
}

// get returns the text of the first child with any of the names.
func (n *node) get(names ...string) string {
	for _, name := range names {
		if c := n.child(name); c != nil && c.text != "" {
			return c.text
		}
	}
	return ""
}

// all returns the children with the given local name, ignoring case.
func (n *node) all(name string) []*node {
	if n == nil {
		return nil
	}
	var out []*node
	for _, c := range n.children {
		if strings.EqualFold(c.name, name) {
			out = append(out, c)
		}
	}
	return out
}

// texts returns the text of every child (string arrays).
func (n *node) texts() []string {
	if n == nil {
		return nil
	}
	var out []string
	for _, c := range n.children {
		if c.text != "" {
			out = append(out, c.text)
		}
	}
	return out
}
