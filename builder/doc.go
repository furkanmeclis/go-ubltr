// Package builder constructs generated UBL-TR Invoice types without replacing them.
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
// Build returns *invoice.Invoice. Money arithmetic uses decimal, not float64.
package builder
