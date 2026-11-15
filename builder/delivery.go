package builder

import (
	"strings"

	"github.com/furkanmeclis/go-ubltr/codelist"
	"github.com/furkanmeclis/go-ubltr/schema/ubltr121/cac"
	"github.com/furkanmeclis/go-ubltr/schema/ubltr121/cbc"
	"github.com/furkanmeclis/go-ubltr/schema/ubltr121/udt"
)

// incotermsScheme is the DeliveryTerms/ID schemeID GİB reads for IHRACAT.
const incotermsScheme = "INCOTERMS"

// DeliveryBuilder constructs a generated [cac.DeliveryType]. It is used for
// document-level delivery ([InvoiceBuilder.Delivery]) and line-level delivery
// ([InvoiceLineBuilder.Delivery]); IHRACAT invoices need the shipment fields.
type DeliveryBuilder struct {
	err error

	date, tracking string
	addr           cac.AddressType
	hasAddr        bool

	incoterm, specialTerms string
	shipmentID             string
	customsID              string
	transportMode          string
	packages               []cac.PackageType
	hasShipmentID          bool
}

// NewDeliveryBuilder returns an empty delivery builder.
func NewDeliveryBuilder() *DeliveryBuilder {
	return &DeliveryBuilder{packages: []cac.PackageType{}}
}

func (b *DeliveryBuilder) fail(field, msg string) {
	if b == nil || b.err != nil {
		return
	}
	b.err = builderErr(field, msg)
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

// Incoterms sets DeliveryTerms/ID with schemeID INCOTERMS ([codelist.DeliveryTermFOB], …).
func (b *DeliveryBuilder) Incoterms(code string) *DeliveryBuilder {
	code = strings.ToUpper(strings.TrimSpace(code))
	if !codelist.ValidDeliveryTerm(code) {
		b.fail("delivery.incoterms", "unknown delivery term "+code)
		return b
	}
	b.incoterm = code
	return b
}

// SpecialTerms sets DeliveryTerms/SpecialTerms (free text next to the INCOTERMS code).
func (b *DeliveryBuilder) SpecialTerms(s string) *DeliveryBuilder {
	b.specialTerms = s
	return b
}

// ShipmentID sets Shipment/ID. An empty ID is written when shipment details
// are present and no ID is set, as in the GİB export samples.
func (b *DeliveryBuilder) ShipmentID(id string) *DeliveryBuilder {
	b.shipmentID = id
	b.hasShipmentID = true
	return b
}

// CustomsID sets Shipment/GoodsItem/RequiredCustomsID (GTİP, the customs
// tariff code). Required on every IHRACAT line.
func (b *DeliveryBuilder) CustomsID(gtip string) *DeliveryBuilder {
	b.customsID = strings.TrimSpace(gtip)
	return b
}

// TransportMode sets Shipment/ShipmentStage/TransportModeCode
// ([codelist.TransportMode1] sea, 3 road, 4 air, …).
func (b *DeliveryBuilder) TransportMode(code string) *DeliveryBuilder {
	code = strings.TrimSpace(code)
	if !codelist.ValidTransportMode(code) {
		b.fail("delivery.transportMode", "unknown transport mode "+code)
		return b
	}
	b.transportMode = code
	return b
}

// Package appends Shipment/TransportHandlingUnit/ActualPackage with ID,
// Quantity (package count), and PackagingTypeCode ([codelist.PackageBX], …).
func (b *DeliveryBuilder) Package(id, packagingTypeCode, quantity string) *DeliveryBuilder {
	packagingTypeCode = strings.TrimSpace(packagingTypeCode)
	if packagingTypeCode != "" && !codelist.ValidPackage(packagingTypeCode) {
		b.fail("delivery.package", "unknown packaging type "+packagingTypeCode)
		return b
	}
	pkg := cac.PackageType{}
	if id != "" {
		v := cbcID(id)
		pkg.ID = &v
	}
	if quantity != "" {
		q := udt.QuantityType{Value: strings.TrimSpace(quantity)}
		pkg.Quantity = &q
	}
	if packagingTypeCode != "" {
		pkg.PackagingTypeCode = &cbc.PackagingTypeCodeType{Value: packagingTypeCode}
	}
	b.packages = append(b.packages, pkg)
	return b
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
	if b.incoterm != "" || b.specialTerms != "" {
		terms := cac.DeliveryTermsType{}
		if b.incoterm != "" {
			id := cbcIDScheme(incotermsScheme, b.incoterm)
			terms.ID = &id
		}
		if b.specialTerms != "" {
			terms.SpecialTerms = &cbc.SpecialTermsType{Value: b.specialTerms}
		}
		d.DeliveryTerms = append(d.DeliveryTerms, terms)
	}
	if b.hasShipmentID || b.customsID != "" || b.transportMode != "" || len(b.packages) > 0 {
		sh := &cac.ShipmentType{ID: cbcID(b.shipmentID)}
		if b.customsID != "" {
			sh.GoodsItem = []cac.GoodsItemType{{
				RequiredCustomsID: &cbc.RequiredCustomsIDType{Value: b.customsID},
			}}
		}
		if b.transportMode != "" {
			sh.ShipmentStage = []cac.ShipmentStageType{{
				TransportModeCode: &cbc.TransportModeCodeType{Value: b.transportMode},
			}}
		}
		if len(b.packages) > 0 {
			sh.TransportHandlingUnit = []cac.TransportHandlingUnitType{{
				ActualPackage: b.packages,
			}}
		}
		d.Shipment = sh
	}
	return d, nil
}

func deliveryHasCustomsID(ds []cac.DeliveryType) bool {
	for _, d := range ds {
		if d.Shipment == nil {
			continue
		}
		for _, g := range d.Shipment.GoodsItem {
			if g.RequiredCustomsID != nil && strings.TrimSpace(g.RequiredCustomsID.Value) != "" {
				return true
			}
		}
	}
	return false
}
