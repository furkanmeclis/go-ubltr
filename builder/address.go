package builder

import (
	"github.com/furkanmeclis/go-ubltr/schema/ubltr121/cac"
	"github.com/furkanmeclis/go-ubltr/schema/ubltr121/cbc"
)

// AddressBuilder constructs a generated [cac.AddressType].
type AddressBuilder struct {
	err error

	street, building, room, subdivision, city, postal, country, countryCode string
}

// NewAddressBuilder returns an empty address builder.
func NewAddressBuilder() *AddressBuilder {
	return &AddressBuilder{}
}

func (b *AddressBuilder) fail(field, msg string) {
	if b == nil || b.err != nil {
		return
	}
	b.err = builderErr(field, msg)
}

// Street sets StreetName.
func (b *AddressBuilder) Street(s string) *AddressBuilder {
	b.street = s
	return b
}

// Building sets BuildingNumber.
func (b *AddressBuilder) Building(s string) *AddressBuilder {
	b.building = s
	return b
}

// Room sets Room.
func (b *AddressBuilder) Room(s string) *AddressBuilder {
	b.room = s
	return b
}

// District sets CitySubdivisionName (ilçe).
func (b *AddressBuilder) District(s string) *AddressBuilder {
	b.subdivision = s
	return b
}

// City sets CityName.
func (b *AddressBuilder) City(s string) *AddressBuilder {
	b.city = s
	return b
}

// PostalZone sets PostalZone.
func (b *AddressBuilder) PostalZone(s string) *AddressBuilder {
	b.postal = s
	return b
}

// Country sets Country/Name.
func (b *AddressBuilder) Country(s string) *AddressBuilder {
	b.country = s
	return b
}

// CountryCode sets Country/IdentificationCode (ISO 3166-1 alpha-2).
func (b *AddressBuilder) CountryCode(s string) *AddressBuilder {
	b.countryCode = s
	return b
}

// Build returns a generated address. City, district, and country are required.
func (b *AddressBuilder) Build() (cac.AddressType, error) {
	if b == nil {
		return cac.AddressType{}, builderErr("address", "nil builder")
	}
	if b.err != nil {
		return cac.AddressType{}, b.err
	}
	if b.city == "" {
		b.fail("city", "required")
	}
	if b.subdivision == "" {
		b.fail("district", "required")
	}
	if b.country == "" {
		b.fail("country", "required")
	}
	if b.err != nil {
		return cac.AddressType{}, b.err
	}
	return cac.AddressType{
		Room:                optRoom(b.room),
		StreetName:          optStreet(b.street),
		BuildingNumber:      optBuilding(b.building),
		CitySubdivisionName: cbc.CitySubdivisionNameType{Value: b.subdivision},
		CityName:            cbc.CityNameType{Value: b.city},
		PostalZone:          optPostal(b.postal),
		Country: cac.CountryType{
			IdentificationCode: optCode(b.countryCode),
			Name:               cbc.NameType{Value: b.country},
		},
	}, nil
}
