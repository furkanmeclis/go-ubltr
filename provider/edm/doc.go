// Package edm is the EDM Bilişim özel entegratör client for the
// vendor-neutral provider contract.
//
// It talks to the EFaturaEDM.svc SOAP service. Request shapes and
// SOAPAction values follow the public WSDL (copies in testdata/wsdl); the
// login, send, fetch and status flows also follow the open-source
// rebasesoftware/edm-efatura client.
//
// Importing the package registers it as "edm":
//
//	import _ "github.com/furkanmeclis/go-ubltr/provider/edm"
//
//	p, err := provider.Open("edm", provider.Config{Username: u, Password: pw, Test: true})
package edm
