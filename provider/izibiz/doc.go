// Package izibiz is the İzibiz özel entegratör client for the
// vendor-neutral provider contract.
//
// It talks to the İzibiz SOAP services (AuthenticationWS, EInvoiceWS,
// EIArchiveWS and EIrsaliyeWS). Request shapes follow the WSDL and XSD
// files published on the İzibiz test system (copies in testdata/wsdl),
// the İzibiz Developer Portal and the izibiz/ws-client-dotnet sample
// client.
//
// Importing the package registers it as "izibiz":
//
//	import _ "github.com/furkanmeclis/go-ubltr/provider/izibiz"
//
//	p, err := provider.Open("izibiz", provider.Config{Username: u, Password: pw, Test: true})
package izibiz
