package builder

import (
	"github.com/furkanmeclis/go-ubltr/schema/ubltr121/cac"
)

// DeliveryBuilder constructs a generated [cac.DeliveryType].
type DeliveryBuilder struct {
	err error

	date, tracking string
	addr           cac.AddressType
	hasAddr        bool
}

// NewDeliveryBuilder returns an empty delivery builder.
func NewDeliveryBuilder() *DeliveryBuilder {
	return &DeliveryBuilder{}
}

func (b *DeliveryBuilder) failErr(field string, err error) {
	if b == nil || b.err != nil || err == nil {
		return
	}
	b.err = builderWrap(field, err)
}

// Date sets ActualDeliveryDate.
func (b *DeliveryBuilder) Date(date string) *DeliveryBuilder {
	b.date = date
	return b
}

// TrackingID sets TrackingID.
func (b *DeliveryBuilder) TrackingID(id string) *DeliveryBuilder {
	b.tracking = id
	return b
}

// Address sets DeliveryAddress.
func (b *DeliveryBuilder) Address(addr cac.AddressType) *DeliveryBuilder {
	b.addr = addr
	b.hasAddr = true
	return b
}

// AddressBuilder builds addr and sets DeliveryAddress.
func (b *DeliveryBuilder) AddressBuilder(ab *AddressBuilder) *DeliveryBuilder {
	addr, err := ab.Build()
	if err != nil {
		b.failErr("delivery.address", err)
		return b
	}
	return b.Address(addr)
}

// Build returns a generated delivery.
func (b *DeliveryBuilder) Build() (cac.DeliveryType, error) {
	if b == nil {
		return cac.DeliveryType{}, builderErr("delivery", "nil builder")
	}
	if b.err != nil {
		return cac.DeliveryType{}, b.err
	}
	d := cac.DeliveryType{
		ActualDeliveryDate: optDate(b.date),
		TrackingID:         optTracking(b.tracking),
		DeliveryTerms:      []cac.DeliveryTermsType{},
	}
	if b.hasAddr {
		addr := b.addr
		d.DeliveryAddress = &addr
	}
	return d, nil
}
