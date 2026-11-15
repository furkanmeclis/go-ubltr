package isnet_test

import (
	"context"
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/go-ubltr/provider"
	"github.com/furkanmeclis/go-ubltr/provider/isnet"
)

// schemaIndex holds the complex types and SOAP actions of a WSDL.
type schemaIndex struct {
	members map[string][]string // complexType -> element names, base first
	actions map[string]bool
}

type xsElement struct {
	Name string `xml:"name,attr"`
}

type xsComplex struct {
	Name     string      `xml:"name,attr"`
	Elements []xsElement `xml:"sequence>element"`
	Ext      struct {
		Base     string      `xml:"base,attr"`
		Elements []xsElement `xml:"sequence>element"`
	} `xml:"complexContent>extension"`
}

type wsdlDoc struct {
	Schemas []struct {
		Types []xsComplex `xml:"complexType"`
	} `xml:"types>schema"`
	Bindings []struct {
		Ops []struct {
			Action struct {
				SoapAction string `xml:"soapAction,attr"`
			} `xml:"operation"`
		} `xml:"operation"`
	} `xml:"binding"`
}

func loadWSDL(t *testing.T, name string) schemaIndex {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "wsdl", name))
	if err != nil {
		t.Fatal(err)
	}
	var d wsdlDoc
	if err := xml.Unmarshal(b, &d); err != nil {
		t.Fatal(err)
	}
	raw := map[string]xsComplex{}
	for _, s := range d.Schemas {
		for _, c := range s.Types {
			if _, dup := raw[c.Name]; !dup {
				raw[c.Name] = c
			}
		}
	}
	idx := schemaIndex{members: map[string][]string{}, actions: map[string]bool{}}
	var resolve func(string) []string
	resolve = func(n string) []string {
		c, ok := raw[n]
		if !ok {
			return nil
		}
		var out []string
		if c.Ext.Base != "" {
			out = append(out, resolve(c.Ext.Base[strings.Index(c.Ext.Base, ":")+1:])...)
		}
		for _, e := range append(c.Elements, c.Ext.Elements...) {
			out = append(out, e.Name)
		}
		return out
	}
	for n := range raw {
		idx.members[n] = resolve(n)
	}
	for _, b := range d.Bindings {
		for _, op := range b.Ops {
			idx.actions[op.Action.SoapAction] = true
		}
	}
	if len(idx.members) == 0 || len(idx.actions) == 0 {
		t.Fatalf("%s: nothing parsed", name)
	}
	return idx
}

// TestRequestsMatchWSDL checks every element this module sends against the
// complex types and SOAP actions of the published WSDLs.
func TestRequestsMatchWSDL(t *testing.T) {
	inv, ab := loadWSDL(t, "InvoiceService.wsdl"), loadWSDL(t, "AddressBookService.wsdl")
	f, srv := newServer(t, func(m string, _ int, c call) string {
		switch m {
		case "SearchAllInvoice":
			return envelope(m, `<a:Invoices><a:Invoice><a:ETTN>u</a:ETTN><a:InvoiceXml>PGEvPg==</a:InvoiceXml><a:Status>Onaylandi</a:Status></a:Invoice></a:Invoices><a:Result>Success</a:Result>`)
		case "SearchArchiveInvoice":
			return envelope(m, `<a:ArchiveInvoices><a:ArchiveInvoice><a:IsCanceled>true</a:IsCanceled><a:InvoiceXml>PGEvPg==</a:InvoiceXml></a:ArchiveInvoice></a:ArchiveInvoices><a:Result>Success</a:Result>`)
		case "GetTaxPayer", "GetCompanyVendor", "GetDocumentViewerLink":
			return envelope(m, `<a:DocumentViewerLink>x</a:DocumentViewerLink><a:Result>Success</a:Result>`)
		}
		return ""
	})
	c := newClient(t, srv.URL, func(c *isnet.Config) { c.VendorNumbers = []string{"B01"} })
	ctx := context.Background()
	day := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	q := provider.ListQuery{From: day, To: day.Add(24 * time.Hour)}
	if _, err := c.ListInbox(ctx, q); err != nil {
		t.Fatal(err)
	}
	if _, err := c.ListEArchiveCancelled(ctx, q); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Fetch(ctx, provider.FetchQuery{UUID: "u", Direction: provider.Inbound}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.CheckUser(ctx, "1"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Vendors(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := c.DocumentViewerLink(ctx, provider.EInvoice, provider.Inbound, "u"); err != nil {
		t.Fatal(err)
	}
	types := map[string]string{
		"SearchAllInvoice":      "SearchAllInvoiceRequest",
		"SearchArchiveInvoice":  "SearchArchiveInvoiceRequest",
		"GetDocumentViewerLink": "DocumentViewerLinkRequest",
		"GetCompanyVendor":      "GetVendorRequest",
		"GetTaxPayer":           "GetTaxPayerRequest",
	}
	nested := map[string]string{"PagingRequest": "PaginationInfoRequest", "ResultSet": "InvoiceResultSet"}
	f.mu.Lock()
	defer f.mu.Unlock()
	seen := map[string]bool{}
	for _, cl := range f.calls {
		idx := inv
		if cl.method == "GetTaxPayer" {
			idx = ab
		}
		seen[cl.method] = true
		if !idx.actions[strings.Trim(cl.action, `"`)] {
			t.Errorf("SOAPAction %s not in WSDL", cl.action)
		}
		for _, l := range cl.leaves {
			parts := strings.Split(l.path, "/")
			if len(parts) < 3 { // method, request
				continue
			}
			local := func(p string) string { return p[strings.Index(p, "}")+1:] }
			owner := types[cl.method]
			if len(parts) == 4 {
				owner = nested[local(parts[2])]
				if owner == "" { // string arrays
					continue
				}
			}
			if len(parts) > 4 {
				continue
			}
			if !strings.HasPrefix(parts[len(parts)-1], "{"+nsModel+"}") {
				t.Errorf("%s: %s is not in the data-contract namespace", cl.method, l.path)
			}
			name := local(parts[len(parts)-1])
			found := false
			for _, m := range idx.members[owner] {
				if m == name {
					found = true
				}
			}
			if !found {
				t.Errorf("%s: %s is not a member of %s", cl.method, name, owner)
			}
		}
	}
	for m := range types {
		if !seen[m] {
			t.Errorf("%s not exercised", m)
		}
	}
}
