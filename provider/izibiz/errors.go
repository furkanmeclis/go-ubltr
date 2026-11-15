package izibiz

import (
	"bytes"
	_ "embed"
	"encoding/xml"
	"errors"
	"io"
	"strings"

	"github.com/furkanmeclis/go-ubltr/provider"
)

//go:embed errors.csv
var errorsCSV string

// Codes is the İzibiz ERROR_CODE table (errors.csv).
var Codes = provider.MustParseCodeTable(Name, errorsCSV)

// faultDetail extracts REQUEST_ERRORType fields from a RequestFault detail.
// EInvoiceWS puts them directly under RequestFault; EIArchiveWS nests them
// in RequestFault/RequestFaultResponse. The first ERROR_CODE found wins.
func faultDetail(detail string) *errorType {
	if strings.TrimSpace(detail) == "" {
		return nil
	}
	dec := xml.NewDecoder(bytes.NewReader([]byte("<d>" + detail + "</d>")))
	dec.Strict = false
	var et errorType
	found := false
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) || err != nil {
			break
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		var dst *string
		switch se.Name.Local {
		case "ERROR_CODE":
			if found {
				continue
			}
			dst = &et.Code
		case "ERROR_SHORT_DES":
			dst = &et.Short
		case "ERROR_LONG_DES":
			dst = &et.Long
		case "INTL_TXN_ID":
			dst = &et.IntlTxnID
		default:
			continue
		}
		var v string
		if err := dec.DecodeElement(&v, &se); err != nil {
			break
		}
		if *dst == "" {
			*dst = strings.TrimSpace(v)
		}
		if dst == &et.Code {
			found = true
		}
	}
	if !found {
		return nil
	}
	return &et
}
