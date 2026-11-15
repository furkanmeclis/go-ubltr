package builder

import (
	"strings"

	"github.com/furkanmeclis/go-ubltr/codelist"
	"github.com/furkanmeclis/go-ubltr/schema/ubltr121/cac"
	"github.com/furkanmeclis/go-ubltr/schema/ubltr121/cbc"
)

// PartyBuilder constructs a generated [cac.PartyType].
type PartyBuilder struct {
	err error

	name, website, taxOffice string
	email, phone, fax        string
	firstName, familyName    string
	nationality              string
	registrationName         string
	companyID                string
	ids                      []cac.PartyIdentificationType
	address                  cac.AddressType
	hasAddress               bool
}

// NewPartyBuilder returns an empty party builder.
func NewPartyBuilder() *PartyBuilder {
	return &PartyBuilder{ids: []cac.PartyIdentificationType{}}
}

func (b *PartyBuilder) fail(field, msg string) {
	if b == nil || b.err != nil {
		return
	}
	b.err = builderErr(field, msg)
}

func (b *PartyBuilder) failErr(field string, err error) {
	if b == nil || b.err != nil || err == nil {
		return
	}
	b.err = builderWrap(field, err)
}

// Name sets PartyName.
func (b *PartyBuilder) Name(s string) *PartyBuilder {
	b.name = s
	return b
}

// VKN appends a VKN PartyIdentification.
func (b *PartyBuilder) VKN(vkn string) *PartyBuilder {
	return b.Identification(codelist.SchemeVKN, vkn)
}

// TCKN appends a TCKN PartyIdentification.
func (b *PartyBuilder) TCKN(tckn string) *PartyBuilder {
	return b.Identification(codelist.SchemeTCKN, tckn)
}

// Identification appends a PartyIdentification with schemeID.
func (b *PartyBuilder) Identification(scheme, value string) *PartyBuilder {
	if scheme == "" || value == "" {
		b.fail("identification", "scheme and value are required")
		return b
	}
	b.ids = append(b.ids, cac.PartyIdentificationType{ID: cbcIDScheme(scheme, value)})
	return b
}

// Address sets PostalAddress from a built address.
func (b *PartyBuilder) Address(addr cac.AddressType) *PartyBuilder {
	b.address = addr
	b.hasAddress = true
	return b
}

// AddressBuilder builds addr and sets PostalAddress.
func (b *PartyBuilder) AddressBuilder(ab *AddressBuilder) *PartyBuilder {
	addr, err := ab.Build()
	if err != nil {
		b.failErr("address", err)
		return b
	}
	return b.Address(addr)
}

// TaxOffice sets PartyTaxScheme/TaxScheme/Name.
func (b *PartyBuilder) TaxOffice(s string) *PartyBuilder {
	b.taxOffice = s
	return b
}

// Website sets WebsiteURI.
func (b *PartyBuilder) Website(s string) *PartyBuilder {
	b.website = s
	return b
}

// Email sets Contact/ElectronicMail.
func (b *PartyBuilder) Email(s string) *PartyBuilder {
	b.email = s
	return b
}

// Phone sets Contact/Telephone.
func (b *PartyBuilder) Phone(s string) *PartyBuilder {
	b.phone = s
	return b
}

// Fax sets Contact/Telefax.
func (b *PartyBuilder) Fax(s string) *PartyBuilder {
	b.fax = s
	return b
}

// Person sets Person FirstName and FamilyName.
func (b *PartyBuilder) Person(first, family string) *PartyBuilder {
	b.firstName = first
	b.familyName = family
	return b
}

// LegalEntity sets PartyLegalEntity RegistrationName (resmi unvan) and,
// when non-empty, CompanyID. IHRACAT requires a registration name on the
// buyer ([InvoiceBuilder.BuyerCustomerBuilder]).
func (b *PartyBuilder) LegalEntity(registrationName, companyID string) *PartyBuilder {
	b.registrationName = registrationName
	b.companyID = companyID
	return b
}

// Nationality sets Person/NationalityID (ISO 3166-1 alpha-2). It is written
// only when Person is set.
func (b *PartyBuilder) Nationality(code string) *PartyBuilder {
	b.nationality = strings.ToUpper(strings.TrimSpace(code))
	return b
}

// Build returns a generated party. At least one identification and an address are required.
func (b *PartyBuilder) Build() (cac.PartyType, error) {
	if b == nil {
		return cac.PartyType{}, builderErr("party", "nil builder")
	}
	if b.err != nil {
		return cac.PartyType{}, b.err
	}
	if len(b.ids) == 0 {
		b.fail("identification", "VKN or TCKN is required")
	}
	if !b.hasAddress {
		b.fail("address", "required")
	}
	if b.err != nil {
		return cac.PartyType{}, b.err
	}

	p := cac.PartyType{
		WebsiteURI:          optURI(b.website),
		PartyIdentification: b.ids,
		PostalAddress:       b.address,
	}
	if b.name != "" {
		p.PartyName = &cac.PartyNameType{Name: cbc.NameType{Value: b.name}}
	}
	if b.taxOffice != "" {
		p.PartyTaxScheme = &cac.PartyTaxSchemeType{
			TaxScheme: cac.TaxSchemeType{Name: optText(b.taxOffice)},
		}
	}
	if b.email != "" || b.phone != "" || b.fax != "" {
		p.Contact = &cac.ContactType{
			Telephone:      optPhone(b.phone),
			Telefax:        optFax(b.fax),
			ElectronicMail: optMail(b.email),
		}
	}
	if b.firstName != "" || b.familyName != "" {
		p.Person = &cac.PersonType{
			FirstName:  cbc.FirstNameType{Value: b.firstName},
			FamilyName: cbc.FamilyNameType{Value: b.familyName},
		}
		if b.nationality != "" {
			p.Person.NationalityID = &cbc.NationalityIDType{Value: b.nationality}
		}
	}
	if b.registrationName != "" || b.companyID != "" {
		le := cac.PartyLegalEntityType{}
		if b.registrationName != "" {
			le.RegistrationName = &cbc.RegistrationNameType{Value: b.registrationName}
		}
		if b.companyID != "" {
			le.CompanyID = &cbc.CompanyIDType{Value: b.companyID}
		}
		p.PartyLegalEntity = []cac.PartyLegalEntityType{le}
	}
	return p, nil
}
