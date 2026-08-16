package builder

import (
	"github.com/furkanmeclis/go-ubltr/schema/ubltr121/cbc"
	"github.com/furkanmeclis/go-ubltr/schema/ubltr121/udt"
)

func amountValue(m Money, round bool) udt.AmountType {
	v := m.String()
	if round {
		v = m.Lexical()
	}
	return udt.AmountType{CurrencyID: m.currency, Value: v}
}

func quantityValue(q Quantity) udt.QuantityType {
	return udt.QuantityType{UnitCode: q.unit, Value: q.String()}
}

func cbcID(v string) cbc.IDType {
	return cbc.IDType{Value: v}
}

func cbcIDScheme(scheme, v string) cbc.IDType {
	return cbc.IDType{SchemeID: scheme, Value: v}
}

func optStreet(s string) *cbc.StreetNameType {
	if s == "" {
		return nil
	}
	v := cbc.StreetNameType{Value: s}
	return &v
}

func optBuilding(s string) *cbc.BuildingNumberType {
	if s == "" {
		return nil
	}
	v := cbc.BuildingNumberType{Value: s}
	return &v
}

func optPostal(s string) *cbc.PostalZoneType {
	if s == "" {
		return nil
	}
	v := cbc.PostalZoneType{Value: s}
	return &v
}

func optRoom(s string) *cbc.RoomType {
	if s == "" {
		return nil
	}
	v := cbc.RoomType{Value: s}
	return &v
}

func optText(s string) *cbc.NameType {
	if s == "" {
		return nil
	}
	v := cbc.NameType{Value: s}
	return &v
}

func optNote(s string) *cbc.NoteType {
	if s == "" {
		return nil
	}
	v := cbc.NoteType{Value: s}
	return &v
}

func optURI(s string) *cbc.WebsiteURIType {
	if s == "" {
		return nil
	}
	v := cbc.WebsiteURIType{Value: s}
	return &v
}

func optPhone(s string) *cbc.TelephoneType {
	if s == "" {
		return nil
	}
	v := cbc.TelephoneType{Value: s}
	return &v
}

func optFax(s string) *cbc.TelefaxType {
	if s == "" {
		return nil
	}
	v := cbc.TelefaxType{Value: s}
	return &v
}

func optMail(s string) *cbc.ElectronicMailType {
	if s == "" {
		return nil
	}
	v := cbc.ElectronicMailType{Value: s}
	return &v
}

func optCode(s string) *cbc.IdentificationCodeType {
	if s == "" {
		return nil
	}
	v := cbc.IdentificationCodeType{Value: s}
	return &v
}

func optDate(s string) *cbc.ActualDeliveryDateType {
	if s == "" {
		return nil
	}
	v := cbc.ActualDeliveryDateType{Value: s}
	return &v
}

func optTracking(s string) *cbc.TrackingIDType {
	if s == "" {
		return nil
	}
	v := cbc.TrackingIDType{Value: s}
	return &v
}

func optPercent(s string) *cbc.PercentType {
	if s == "" {
		return nil
	}
	v := cbc.PercentType{Value: s}
	return &v
}

func optExemptionCode(s string) *cbc.TaxExemptionReasonCodeType {
	if s == "" {
		return nil
	}
	v := cbc.TaxExemptionReasonCodeType{Value: s}
	return &v
}

func optExemptionReason(s string) *cbc.TaxExemptionReasonType {
	if s == "" {
		return nil
	}
	v := cbc.TaxExemptionReasonType{Value: s}
	return &v
}

func optMultiplier(s string) *cbc.MultiplierFactorNumericType {
	if s == "" {
		return nil
	}
	v := cbc.MultiplierFactorNumericType{Value: s}
	return &v
}

func optDueDate(s string) *cbc.PaymentDueDateType {
	if s == "" {
		return nil
	}
	v := cbc.PaymentDueDateType{Value: s}
	return &v
}

func optInstruction(s string) *cbc.InstructionNoteType {
	if s == "" {
		return nil
	}
	v := cbc.InstructionNoteType{Value: s}
	return &v
}

func parseAmount(a udt.AmountType) (Money, error) {
	return MoneyIn(a.CurrencyID, a.Value)
}
