package builder

import "github.com/furkanmeclis/go-ubltr/schema/ubltr121/ext"

const (
	ublVersionTR      = "2.1"
	customizationTR12 = "TR1.2"
	// extensionContentPlaceholder is one ##other element. UBL requires
	// ExtensionContent to contain a child outside the ext namespace.
	extensionContentPlaceholder = `<UBLTR xmlns="urn:ubltr:extension"/>`
)

func ublExtensions() ext.UBLExtensionsType {
	return ext.UBLExtensionsType{
		UBLExtension: []ext.UBLExtensionType{{
			ExtensionContent: ext.ExtensionContentType{
				InnerXML: extensionContentPlaceholder,
			},
		}},
	}
}

func ptrExtensions() *ext.UBLExtensionsType {
	e := ublExtensions()
	return &e
}

func ptrOf[T any](v T) *T { return &v }
