package builder

import (
	"github.com/furkanmeclis/go-ubltr/codelist"
	"github.com/furkanmeclis/go-ubltr/schema/ubltr121/cac"
	"github.com/furkanmeclis/go-ubltr/schema/ubltr121/cbc"
	"github.com/furkanmeclis/go-ubltr/schema/ubltr121/invoice"
)

// ReturnOption changes how [NewReturnInvoiceFrom] derives the return invoice.
type ReturnOption func(*returnConfig)

type returnConfig struct {
	typ         string
	lineIDs     map[string]struct{}
	sameParties bool
}

// ReturnType overrides the InvoiceTypeCode of the return invoice (default:
// YTBIADE for YTBSATIS, YTBISTISNA, and YTBTEVKIFAT; IADE otherwise,
// including TEVKIFAT, so copied withholding lines still pass GİB Schematron).
func ReturnType(code string) ReturnOption {
	return func(c *returnConfig) { c.typ = code }
}

// ReturnLines keeps only the original lines whose cbc:ID is in ids (partial return).
func ReturnLines(ids ...string) ReturnOption {
	return func(c *returnConfig) {
		if c.lineIDs == nil {
			c.lineIDs = map[string]struct{}{}
		}
		for _, id := range ids {
			c.lineIDs[id] = struct{}{}
		}
	}
}

// ReturnSameParties keeps the original supplier and customer instead of
// swapping them.
func ReturnSameParties() ReturnOption {
	return func(c *returnConfig) { c.sameParties = true }
}

// NewReturnInvoiceFrom returns an [InvoiceBuilder] for a return (İade) of
// orig. In Türkiye a return is an Invoice issued by the party sending the
// goods back, so the original customer becomes the supplier and the original
// supplier becomes the customer (see [ReturnSameParties]). Lines, currency,
// and exchange rate are copied (withholding lines keep their
// WithholdingTaxTotal), the type becomes IADE (YTBIADE for YTB invoices, see
// [ReturnType]), the profile becomes TEMELFATURA when the
// original profile does not allow IADE (TICARIFATURA, IHRACAT, …), and a
// BillingReference with DocumentTypeCode IADE points to orig. Set ID and
// IssueDate on the returned builder; a new UUID is generated on Build unless
// UUID is called.
func NewReturnInvoiceFrom(orig *invoice.Invoice, opts ...ReturnOption) *InvoiceBuilder {
	b := NewInvoiceBuilder()
	if orig == nil {
		b.fail("invoice", "nil invoice")
		return b
	}
	cfg := returnConfig{}
	for _, o := range opts {
		if o != nil {
			o(&cfg)
		}
	}
	typ := cfg.typ
	if typ == "" {
		typ = returnTypeFor(orig.InvoiceTypeCode.Value)
	}

	profile := orig.ProfileID.Value
	if typ == codelist.InvoiceIADE && !iadeProfile(profile) {
		profile = codelist.ProfileTEMELFATURA
	}

	b.Profile(profile).Type(typ).Currency(orig.DocumentCurrencyCode.Value)
	if orig.PricingExchangeRate != nil {
		er := *orig.PricingExchangeRate
		b.exchange = &er
	}
	supplier, customer := orig.AccountingCustomerParty.Party, orig.AccountingSupplierParty.Party
	if cfg.sameParties {
		supplier, customer = customer, supplier
	}
	b.Supplier(supplier).Customer(customer)

	for _, line := range orig.InvoiceLine {
		if cfg.lineIDs != nil {
			if _, ok := cfg.lineIDs[line.ID.Value]; !ok {
				continue
			}
		}
		b.lines = append(b.lines, copyLine(line))
	}
	if len(b.lines) == 0 {
		b.fail("line", "no lines selected from the returned invoice")
	}
	return b.ReturnOf(orig)
}

// returnTypeFor picks the default return type. Withholding returns use IADE /
// YTBIADE rather than TEVKIFATIADE / YTBTEVKIFATIADE because the vendored GİB
// Schematron (GeneralWithholdingTaxTotalCheck) rejects cac:WithholdingTaxTotal
// on the latter two.
func returnTypeFor(orig string) string {
	switch orig {
	case codelist.InvoiceYTBSATIS, codelist.InvoiceYTBISTISNA, codelist.InvoiceYTBTEVKIFAT:
		return codelist.InvoiceYTBIADE
	default:
		return codelist.InvoiceIADE
	}
}

// iadeProfile reports whether GİB Schematron allows InvoiceTypeCode IADE on profile.
func iadeProfile(profile string) bool {
	switch profile {
	case codelist.ProfileTEMELFATURA, codelist.ProfileEARSIVFATURA, codelist.ProfileILACTIBBICIHAZ,
		codelist.ProfileYATIRIMTESVIK, codelist.ProfileIDIS, codelist.ProfileKAMU:
		return true
	}
	return false
}

// copyLine copies the slices a builder may append to so the original
// invoice is not shared with the return.
func copyLine(l cac.InvoiceLineType) cac.InvoiceLineType {
	l.Note = append([]cbc.NoteType(nil), l.Note...)
	l.Delivery = append([]cac.DeliveryType(nil), l.Delivery...)
	l.AllowanceCharge = append([]cac.AllowanceChargeType(nil), l.AllowanceCharge...)
	l.WithholdingTaxTotal = append([]cac.TaxTotalType(nil), l.WithholdingTaxTotal...)
	if l.TaxTotal != nil {
		t := *l.TaxTotal
		t.TaxSubtotal = append([]cac.TaxSubtotalType(nil), t.TaxSubtotal...)
		l.TaxTotal = &t
	}
	return l
}
