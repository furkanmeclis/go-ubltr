// Package render turns UBL XML into HTML (and, with a PDF adapter, into PDF).
//
// This module is separate from github.com/furkanmeclis/go-ubltr so that
// XML production and validation stay free of rendering engines. Primary
// input is []byte XML; generated Invoice types are not required.
package render
