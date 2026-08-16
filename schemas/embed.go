// Package schemas embeds the official GİB UBL-TR 1.2.1 and e-Arşiv XSD files.
package schemas

import "embed"

// FS contains vendored GİB UBL-TR 1.2.1 XSD, Schematron, envelope, and
// e-Arşiv report schema files.
//
// UBL-TR and e-Arşiv schema definitions are the source of truth. See NOTICE
// for redistribution terms.
//
//go:embed ubltr-1.2.1/xsdrt ubltr-1.2.1/schematron ubltr-1.2.1/envelope earchive
var FS embed.FS
