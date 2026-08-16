// Package schemas embeds the official GİB UBL-TR 1.2.1 XSD and Schematron files.
package schemas

import "embed"

// FS contains vendored GİB UBL-TR 1.2.1 XSD and Schematron files.
//
// UBL-TR schema definitions are the source of truth. See NOTICE for
// redistribution terms.
//
//go:embed ubltr-1.2.1/xsdrt ubltr-1.2.1/schematron ubltr-1.2.1/envelope
var FS embed.FS
