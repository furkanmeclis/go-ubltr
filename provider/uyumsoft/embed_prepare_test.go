package uyumsoft

import (
	"bytes"
	"testing"
)

const unsignedDoc = `<?xml version="1.0" encoding="UTF-8"?>
<Invoice xmlns="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2" xmlns:cac="urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2" xmlns:cbc="urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2" xmlns:ext="urn:oasis:names:specification:ubl:schema:xsd:CommonExtensionComponents-2">` +
	`<ext:UBLExtensions><ext:UBLExtension><ext:ExtensionContent><UBLTR xmlns="urn:ubltr:extension"/></ext:ExtensionContent></ext:UBLExtension></ext:UBLExtensions>` +
	`<cbc:ID>GUB2026000000001</cbc:ID>` +
	`<cac:Signature><cbc:ID schemeID="VKN_TCKN">1234567890</cbc:ID></cac:Signature>` +
	`<cac:AccountingSupplierParty/></Invoice>`

func TestEmbedInvoicePreparesUnsigned(t *testing.T) {
	e, err := embedInvoice([]byte(unsignedDoc))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(e.Inner, []byte("UBLTR")) {
		t.Errorf("placeholder kept:\n%s", e.Inner)
	}
	if bytes.Contains(e.Inner, []byte("cac:Signature")) {
		t.Errorf("builder cac:Signature kept:\n%s", e.Inner)
	}
	for _, want := range []string{"<ext:ExtensionContent></ext:ExtensionContent>", "<cbc:ID>GUB2026000000001</cbc:ID>", "<cac:AccountingSupplierParty/>"} {
		if !bytes.Contains(e.Inner, []byte(want)) {
			t.Errorf("lost %s:\n%s", want, e.Inner)
		}
	}
}

func TestEmbedInvoiceKeepsSignedUnchanged(t *testing.T) {
	signed := bytes.Replace([]byte(unsignedDoc), []byte(`<UBLTR xmlns="urn:ubltr:extension"/>`),
		[]byte(`<ds:Signature xmlns:ds="http://www.w3.org/2000/09/xmldsig#" Id="S"/>`), 1)
	e, err := embedInvoice(signed)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(e.Inner, []byte("<cac:Signature>")) || !bytes.Contains(e.Inner, []byte("<ds:Signature")) {
		t.Errorf("signed document changed:\n%s", e.Inner)
	}
}
