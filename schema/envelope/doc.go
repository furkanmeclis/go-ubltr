// Package envelope contains Go types generated from the GİB envelope XSDs
// (schemas/ubltr-1.2.1/envelope): the UN/CEFACT Standard Business Document
// Header and the GİB ef:Package.
//
// Documents inside Package/Elements/ElementList are kept as raw XML in
// ElementList.InnerXML. Use ubltr.NewEnvelope and ubltr.EnvelopeDocuments to
// build and read envelopes with typed UBL-TR documents.
package envelope
