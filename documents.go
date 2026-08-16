package ubltr

import (
	"crypto/rand"
	"fmt"

	"github.com/furkanmeclis/go-ubltr/schema/earchive"
	"github.com/furkanmeclis/go-ubltr/schema/ubltr121/applicationresponse"
	"github.com/furkanmeclis/go-ubltr/schema/ubltr121/cac"
	"github.com/furkanmeclis/go-ubltr/schema/ubltr121/cbc"
	"github.com/furkanmeclis/go-ubltr/schema/ubltr121/creditnote"
	"github.com/furkanmeclis/go-ubltr/schema/ubltr121/despatchadvice"
	"github.com/furkanmeclis/go-ubltr/schema/ubltr121/ext"
	"github.com/furkanmeclis/go-ubltr/schema/ubltr121/invoice"
	"github.com/furkanmeclis/go-ubltr/schema/ubltr121/receiptadvice"
)

// NewCreditNote returns an empty UBL-TR CreditNote document.
func NewCreditNote() *creditnote.CreditNote {
	return &creditnote.CreditNote{}
}

// NewApplicationResponse returns an empty UBL-TR ApplicationResponse document.
func NewApplicationResponse() *applicationresponse.ApplicationResponse {
	return &applicationresponse.ApplicationResponse{}
}

// NewDespatchAdvice returns an empty UBL-TR DespatchAdvice document.
func NewDespatchAdvice() *despatchadvice.DespatchAdvice {
	return &despatchadvice.DespatchAdvice{}
}

// NewReceiptAdvice returns an empty UBL-TR ReceiptAdvice document.
func NewReceiptAdvice() *receiptadvice.ReceiptAdvice {
	return &receiptadvice.ReceiptAdvice{}
}

// NewEArchiveReport returns an empty GİB e-Arşiv *report* (eArsivRaporu).
// This is not a UBL Invoice. e-Arşiv invoices stay UBL Invoice with
// ProfileID EARSIVFATURA.
func NewEArchiveReport() *earchive.EArsivRaporu {
	return &earchive.EArsivRaporu{}
}

// NewCreditNoteFromInvoice copies an Invoice onto a CreditNote: parties, lines
// (InvoicedQuantity → CreditedQuantity), totals, tax, and a BillingReference
// to the original invoice. A new UUID is generated. The caller should set a
// distinct document ID before sending.
func NewCreditNoteFromInvoice(inv *invoice.Invoice) (*creditnote.CreditNote, error) {
	if inv == nil {
		return nil, fmt.Errorf("nil invoice")
	}

	uuid, err := newDocumentUUID()
	if err != nil {
		return nil, err
	}

	cn := NewCreditNote()
	cn.UBLExtensions = placeholderExtensions()
	cn.UBLVersionID = ptr(inv.UBLVersionID)
	cn.CustomizationID = ptr(inv.CustomizationID)
	cn.ProfileID = ptr(inv.ProfileID)
	cn.ID = inv.ID
	cn.CopyIndicator = ptr(inv.CopyIndicator)
	u := cbc.UUIDType{Value: uuid}
	cn.UUID = &u
	cn.IssueDate = inv.IssueDate
	cn.IssueTime = inv.IssueTime
	if inv.InvoiceTypeCode.Value != "" {
		cn.CreditNoteTypeCode = &cbc.CreditNoteTypeCodeType{Value: inv.InvoiceTypeCode.Value}
	}
	cn.Note = inv.Note
	cn.DocumentCurrencyCode = ptr(inv.DocumentCurrencyCode)
	cn.TaxCurrencyCode = inv.TaxCurrencyCode
	cn.PricingCurrencyCode = inv.PricingCurrencyCode
	cn.PaymentCurrencyCode = inv.PaymentCurrencyCode
	cn.PaymentAlternativeCurrencyCode = inv.PaymentAlternativeCurrencyCode
	cn.AccountingCost = inv.AccountingCost
	cn.LineCountNumeric = ptr(inv.LineCountNumeric)
	if inv.InvoicePeriod != nil {
		cn.InvoicePeriod = []cac.PeriodType{*inv.InvoicePeriod}
	}
	cn.OrderReference = inv.OrderReference
	cn.BillingReference = append([]cac.BillingReferenceType{}, inv.BillingReference...)
	cn.BillingReference = append(cn.BillingReference, cac.BillingReferenceType{
		InvoiceDocumentReference: &cac.DocumentReferenceType{
			ID:        inv.ID,
			IssueDate: inv.IssueDate,
		},
	})
	cn.DespatchDocumentReference = inv.DespatchDocumentReference
	cn.ReceiptDocumentReference = inv.ReceiptDocumentReference
	cn.ContractDocumentReference = inv.ContractDocumentReference
	cn.AdditionalDocumentReference = inv.AdditionalDocumentReference
	cn.OriginatorDocumentReference = inv.OriginatorDocumentReference
	cn.Signature = inv.Signature
	cn.AccountingSupplierParty = inv.AccountingSupplierParty
	cn.AccountingCustomerParty = inv.AccountingCustomerParty
	cn.BuyerCustomerParty = inv.BuyerCustomerParty
	cn.SellerSupplierParty = inv.SellerSupplierParty
	cn.TaxRepresentativeParty = inv.TaxRepresentativeParty
	cn.Delivery = inv.Delivery
	cn.PaymentMeans = inv.PaymentMeans
	if inv.PaymentTerms != nil {
		cn.PaymentTerms = []cac.PaymentTermsType{*inv.PaymentTerms}
	}
	cn.AllowanceCharge = inv.AllowanceCharge
	cn.TaxExchangeRate = inv.TaxExchangeRate
	cn.PricingExchangeRate = inv.PricingExchangeRate
	cn.PaymentExchangeRate = inv.PaymentExchangeRate
	cn.PaymentAlternativeExchangeRate = inv.PaymentAlternativeExchangeRate
	cn.TaxTotal = inv.TaxTotal
	cn.LegalMonetaryTotal = inv.LegalMonetaryTotal
	cn.CreditNoteLine = make([]cac.CreditNoteLineType, 0, len(inv.InvoiceLine))
	for _, line := range inv.InvoiceLine {
		cn.CreditNoteLine = append(cn.CreditNoteLine, creditNoteLineFromInvoice(line))
	}
	return cn, nil
}

func creditNoteLineFromInvoice(line cac.InvoiceLineType) cac.CreditNoteLineType {
	qty := cbc.CreditedQuantityType(line.InvoicedQuantity)
	extAmt := line.LineExtensionAmount
	item := line.Item
	price := line.Price
	out := cac.CreditNoteLineType{
		ID:                    line.ID,
		Note:                  line.Note,
		CreditedQuantity:      &qty,
		LineExtensionAmount:   &extAmt,
		OrderLineReference:    line.OrderLineReference,
		DespatchLineReference: line.DespatchLineReference,
		ReceiptLineReference:  line.ReceiptLineReference,
		Delivery:              line.Delivery,
		AllowanceCharge:       line.AllowanceCharge,
		Item:                  &item,
		Price:                 &price,
	}
	if line.TaxTotal != nil {
		out.TaxTotal = []cac.TaxTotalType{*line.TaxTotal}
	}
	return out
}

func ptr[T any](v T) *T { return &v }

func placeholderExtensions() *ext.UBLExtensionsType {
	e := ext.UBLExtensionsType{
		UBLExtension: []ext.UBLExtensionType{{
			ExtensionContent: ext.ExtensionContentType{
				InnerXML: `<UBLTR xmlns="urn:ubltr:extension"/>`,
			},
		}},
	}
	return &e
}

func newDocumentUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("uuid: %w", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%X-%X-%X-%X-%X", b[0:4], b[4:6], b[6:8], b[8:10], b[10:]), nil
}
