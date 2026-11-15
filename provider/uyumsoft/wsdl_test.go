package uyumsoft

import (
	"encoding/xml"
	"os"
	"testing"

	"github.com/furkanmeclis/go-ubltr/provider"
)

type wsdlDoc struct {
	Types struct {
		Schemas []struct {
			Target      string `xml:"targetNamespace,attr"`
			SimpleTypes []struct {
				Name   string `xml:"name,attr"`
				Values []struct {
					Value string `xml:"value,attr"`
				} `xml:"restriction>enumeration"`
			} `xml:"simpleType"`
		} `xml:"schema"`
	} `xml:"types"`
	Bindings []struct {
		Operations []struct {
			Name   string `xml:"name,attr"`
			Action struct {
				Value string `xml:"soapAction,attr"`
			} `xml:"operation"`
		} `xml:"operation"`
	} `xml:"binding"`
	Ports []struct {
		Address struct {
			Location string `xml:"location,attr"`
		} `xml:"address"`
	} `xml:"service>port"`
}

func loadWSDL(t *testing.T, path string) *wsdlDoc {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var d wsdlDoc
	if err := xml.Unmarshal(b, &d); err != nil {
		t.Fatal(err)
	}
	return &d
}

func wsdlEnum(t *testing.T, path, name string) []string {
	t.Helper()
	for _, s := range loadWSDL(t, path).Types.Schemas {
		if s.Target != nsTempuri {
			continue
		}
		for _, st := range s.SimpleTypes {
			if st.Name == name {
				var out []string
				for _, v := range st.Values {
					out = append(out, v.Value)
				}
				return out
			}
		}
	}
	t.Fatalf("enum %s not in %s", name, path)
	return nil
}

// usedOps lists the operations each provider operation calls.
var usedOps = map[provider.Operation][]string{
	provider.OpSend:           {"SendInvoice"},
	provider.OpStatus:         {"QueryOutboxInvoiceStatus", "QueryInboxInvoiceStatus"},
	provider.OpListInbox:      {"GetInboxInvoiceList"},
	provider.OpListOutbox:     {"GetOutboxInvoiceList"},
	provider.OpFetch:          {"GetOutboxInvoiceData", "GetOutboxInvoicePdf", "GetOutboxInvoiceView", "GetInboxInvoiceData", "GetInboxInvoicePdf", "GetInboxInvoiceView"},
	provider.OpRespond:        {"SendDocumentResponse"},
	provider.OpCancelEArchive: {"CancelEArchiveInvoice"},
	provider.OpCheckUser:      {"IsEInvoiceUser", "GetUserAliasses"},
	provider.OpMarkTaken:      {"SetInvoicesTaken"},
	provider.OpCredits:        {"GetCustomerCreditInfo"},
}

// Every operation behind a declared capability exists in the vendor's
// WSDL with the SOAPAction the preset computes, and the preset's live
// endpoint is the WSDL service address.
func TestPresetsMatchWSDL(t *testing.T) {
	for _, tc := range []struct {
		p    Preset
		path string
	}{
		{Uyumsoft, "testdata/Integration.wsdl"},
		{Muhasebeizleme, "testdata/BasicIntegration.wsdl"},
	} {
		d := loadWSDL(t, tc.path)
		actions := map[string]string{}
		for _, b := range d.Bindings {
			for _, o := range b.Operations {
				actions[o.Name] = o.Action.Value
			}
		}
		ops := []string{"TestConnection"}
		for _, list := range tc.p.Capabilities.Operations {
			for _, op := range list {
				ops = append(ops, usedOps[op]...)
			}
		}
		for _, op := range ops {
			got, ok := actions[op]
			if !ok {
				t.Errorf("%s: %s not in WSDL", tc.p.Name, op)
				continue
			}
			if want := tc.p.action(op); got != want {
				t.Errorf("%s: %s action %s, preset computes %s", tc.p.Name, op, got, want)
			}
		}
		var locs []string
		for _, p := range d.Ports {
			locs = append(locs, p.Address.Location)
		}
		if len(locs) == 0 || locs[0] != tc.p.LiveURL && locs[0] != tc.p.TestURL {
			t.Errorf("%s: WSDL address %v does not match preset", tc.p.Name, locs)
		}
	}
}
