// Package builder constructs generated UBL-TR document types without replacing them.
//
//	inv, err := builder.NewInvoiceBuilder().
//	    Profile(codelist.ProfileTemelFatura).
//	    Type(codelist.InvoiceTypeSatis).
//	    ID("GIB20260000000001").
//	    IssueDate("2026-03-01").
//	    Supplier(supplier).
//	    Customer(customer).
//	    Line(line).
//	    Build()
//
// Build returns generated types (*invoice.Invoice, *creditnote.CreditNote, …).
// Money arithmetic uses decimal, not float64.
package builder
