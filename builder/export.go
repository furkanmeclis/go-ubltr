package builder

const (
	// ExportCustomerVKN is the VKN of the Ministry of Trade customs IT
	// department, the AccountingCustomerParty of every IHRACAT invoice.
	ExportCustomerVKN = "1460415308"
	// ExportCustomerName is the PartyName GİB samples use for [ExportCustomerVKN].
	ExportCustomerName = "Gümrük ve Ticaret Bakanlığı Gümrükler Genel Müdürlüğü- Bilgi İşlem Dairesi Başkanlığı"
	// PartyTypeScheme is the PartyIdentification schemeID for BuyerCustomerParty roles.
	PartyTypeScheme = "PARTYTYPE"
	// PartyTypeExport is the BuyerCustomerParty identification value on IHRACAT.
	PartyTypeExport = "EXPORT"
	// PartyTypeTaxFree is the BuyerCustomerParty identification value on YOLCUBERABERFATURA.
	PartyTypeTaxFree = "TAXFREE"
)

// NewExportCustomerBuilder returns a party builder for the IHRACAT
// AccountingCustomerParty: [ExportCustomerVKN] with [ExportCustomerName] and
// an Ankara address. Call AddressBuilder to replace the address.
func NewExportCustomerBuilder() *PartyBuilder {
	return NewPartyBuilder().
		VKN(ExportCustomerVKN).
		Name(ExportCustomerName).
		AddressBuilder(NewAddressBuilder().
			District("Ulus").
			City("Ankara").
			Country("Türkiye").
			CountryCode("TR"))
}

// NewExportBuyerBuilder returns a party builder for the IHRACAT
// BuyerCustomerParty (the foreign buyer) with PARTYTYPE=EXPORT
// identification. GİB also requires LegalEntity (registration name) and an
// address; set them on the returned builder.
func NewExportBuyerBuilder() *PartyBuilder {
	return NewPartyBuilder().Identification(PartyTypeScheme, PartyTypeExport)
}

// NewTaxFreeBuyerBuilder returns a party builder for the YOLCUBERABERFATURA
// BuyerCustomerParty with PARTYTYPE=TAXFREE identification. Set Person,
// Nationality, and an address on the returned builder.
func NewTaxFreeBuyerBuilder() *PartyBuilder {
	return NewPartyBuilder().Identification(PartyTypeScheme, PartyTypeTaxFree)
}
