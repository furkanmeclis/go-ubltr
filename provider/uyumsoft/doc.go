// Package uyumsoft is a provider.Provider for the Uyumsoft e-Fatura /
// e-Arşiv "Integration" SOAP service and for services that expose the
// same contract, such as muhasebeizleme's BasicIntegration service.
//
// The request and response types are hand-written from the published
// WSDL files (kept in testdata/) and cover only the operations this
// module calls. Vendor differences (authentication, SOAPAction contract
// name, a few parameter names) are described by a [Preset].
//
//	c, err := uyumsoft.New(uyumsoft.Config{
//		Username: "user", Password: "secret", Test: true,
//	})
//	res, err := c.Send(ctx, &provider.Document{XML: ubl})
//
// Importing the package registers "uyumsoft" and "muhasebeizleme" with
// provider.Register.
package uyumsoft
