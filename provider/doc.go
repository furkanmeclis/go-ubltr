// Package provider defines a vendor-neutral contract for sending and
// receiving UBL-TR documents through GİB özel entegratör services.
//
// The package carries no vendor code and no dependency on the core
// go-ubltr module. Each integrator lives in its own nested module under
// provider/<name> and implements only the operations it really offers.
// Callers discover those with [Provider.Capabilities] and with interface
// assertions ([Sender], [InboxLister], …) or with [Supports].
//
// Errors returned by integrator modules are [*Error] values that keep the
// raw vendor code and message, the GİB envelope status code when one is
// known, and a normalized [Kind] usable with errors.Is:
//
//	if errors.Is(err, provider.ErrQuotaExceeded) { … }
//
// Integrator-specific code lists are data ([CodeTable]) so they can be
// extended without changing behaviour elsewhere.
package provider
