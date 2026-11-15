package codelist

// Common aliases for generated identifiers. The full GİB lists are in generated.go.
const (
	ProfileTemelFatura         = ProfileTEMELFATURA
	ProfileTicariFatura        = ProfileTICARIFATURA
	ProfileEArsivFatura        = ProfileEARSIVFATURA
	ProfileTemelIrsaliye       = ProfileTEMELIRSALIYE
	ProfileIhracat             = ProfileIHRACAT
	InvoiceTypeSatis           = InvoiceSATIS
	InvoiceTypeIade            = InvoiceIADE
	InvoiceTypeTevkifat        = InvoiceTEVKIFAT
	InvoiceTypeTevkifatIade    = InvoiceTEVKIFATIADE
	InvoiceTypeIstisna         = InvoiceISTISNA
	TaxKDV                     = Tax0015
	TRY                        = CurrencyTRY
	USD                        = CurrencyUSD
	EUR                        = CurrencyEUR
	GBP                        = CurrencyGBP
	PaymentMeansCash           = PaymentMeans10
	PaymentMeansCreditTransfer = PaymentMeans30
	PaymentMeansBankAccount    = PaymentMeans42
	PaymentMeansBankCard       = PaymentMeans48
	ResponseKabul              = ResponseKABUL
	ResponseRed                = ResponseRED
)

// Trailer scheme IDs moved from LicensePlateIDSchemeIDType to
// TransportEquipmentIDSchemeIDType in the GİB e-Fatura package of
// 24 August 2026. They now belong on
// cac:TransportHandlingUnit/cac:TransportEquipment/cbc:ID, not on
// cbc:LicensePlateID, and ValidPlateScheme no longer accepts them.
//
// Deprecated: use the TrailerScheme* constants.
const (
	PlateSchemeDORSE             = TrailerSchemeDORSE
	PlateSchemeDORSEPLAKA        = TrailerSchemeDORSEPLAKA
	PlateSchemeYABANCIDORSE      = TrailerSchemeYABANCIDORSE
	PlateSchemeYABANCIDORSEPLAKA = TrailerSchemeYABANCIDORSEPLAKA
)
