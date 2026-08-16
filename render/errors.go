package render

import "errors"

var (
	// ErrInvalidXML is returned when the source document is not well-formed XML.
	ErrInvalidXML = errors.New("invalid xml")
	// ErrStylesheet is returned when the stylesheet cannot be compiled.
	ErrStylesheet = errors.New("invalid stylesheet")
	// ErrTransform is returned when a compiled stylesheet fails on a document.
	ErrTransform = errors.New("transform failed")
	// ErrPDFRender is returned when HTML cannot be printed to PDF.
	ErrPDFRender = errors.New("pdf render failed")
)
