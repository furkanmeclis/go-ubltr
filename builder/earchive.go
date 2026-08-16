package builder

import (
	"strings"

	"github.com/furkanmeclis/go-ubltr/schema/earchive"
)

// EArchiveReportBuilder constructs a generated e-Arşiv *report* (eArsivRaporu).
// It is not an Invoice builder. e-Arşiv invoices stay UBL Invoice with
// ProfileID EARSIVFATURA.
type EArchiveReportBuilder struct {
	err error

	version, raporNo string
	mukellefVKN      string
	mukellefTCKN     string
	hazirlayanVKN    string
	hazirlayanTCKN   string
	donemStart       string
	donemEnd         string
	bolumStart       string
	bolumEnd         string
	bolumNo          string
	signatureInner   string
	invoices         []earchive.Fatura
}

// NewEArchiveReportBuilder returns an empty e-Arşiv report builder.
func NewEArchiveReportBuilder() *EArchiveReportBuilder {
	return &EArchiveReportBuilder{
		invoices: []earchive.Fatura{},
	}
}

func (b *EArchiveReportBuilder) fail(field, msg string) {
	if b == nil || b.err != nil {
		return
	}
	b.err = builderErr(field, msg)
}

// Version sets baslik/versiyon (for example "1.0").
func (b *EArchiveReportBuilder) Version(v string) *EArchiveReportBuilder {
	b.version = strings.TrimSpace(v)
	return b
}

// MukellefVKN sets the taxpayer as a VKN.
func (b *EArchiveReportBuilder) MukellefVKN(vkn string) *EArchiveReportBuilder {
	b.mukellefVKN = strings.TrimSpace(vkn)
	b.mukellefTCKN = ""
	return b
}

// MukellefTCKN sets the taxpayer as a TCKN.
func (b *EArchiveReportBuilder) MukellefTCKN(tckn string) *EArchiveReportBuilder {
	b.mukellefTCKN = strings.TrimSpace(tckn)
	b.mukellefVKN = ""
	return b
}

// HazirlayanVKN sets the preparer as a VKN.
func (b *EArchiveReportBuilder) HazirlayanVKN(vkn string) *EArchiveReportBuilder {
	b.hazirlayanVKN = strings.TrimSpace(vkn)
	b.hazirlayanTCKN = ""
	return b
}

// HazirlayanTCKN sets the preparer as a TCKN.
func (b *EArchiveReportBuilder) HazirlayanTCKN(tckn string) *EArchiveReportBuilder {
	b.hazirlayanTCKN = strings.TrimSpace(tckn)
	b.hazirlayanVKN = ""
	return b
}

// ReportNo sets raporNo. Generated on Build when empty.
func (b *EArchiveReportBuilder) ReportNo(id string) *EArchiveReportBuilder {
	b.raporNo = strings.TrimSpace(id)
	return b
}

// Period sets donemBaslangicTarihi and donemBitisTarihi (YYYY-MM-DD).
func (b *EArchiveReportBuilder) Period(start, end string) *EArchiveReportBuilder {
	b.donemStart = strings.TrimSpace(start)
	b.donemEnd = strings.TrimSpace(end)
	return b
}

// Section sets bolumBaslangicTarihi, bolumBitisTarihi, and bolumNo.
func (b *EArchiveReportBuilder) Section(start, end, no string) *EArchiveReportBuilder {
	b.bolumStart = strings.TrimSpace(start)
	b.bolumEnd = strings.TrimSpace(end)
	b.bolumNo = strings.TrimSpace(no)
	return b
}

// SignatureInnerXML sets the inner XML of ds:Signature (not a generated XAdES model).
func (b *EArchiveReportBuilder) SignatureInnerXML(inner string) *EArchiveReportBuilder {
	b.signatureInner = strings.TrimSpace(inner)
	return b
}

// Invoice appends a fatura row.
func (b *EArchiveReportBuilder) Invoice(f earchive.Fatura) *EArchiveReportBuilder {
	b.invoices = append(b.invoices, f)
	return b
}

// Build returns a generated *earchive.EArsivRaporu.
func (b *EArchiveReportBuilder) Build() (*earchive.EArsivRaporu, error) {
	if b == nil {
		return nil, builderErr("builder", "nil")
	}
	if b.err != nil {
		return nil, b.err
	}
	if b.version == "" {
		b.fail("version", "required")
	}
	if b.mukellefVKN == "" && b.mukellefTCKN == "" {
		b.fail("mukellef", "required")
	}
	if b.hazirlayanVKN == "" && b.hazirlayanTCKN == "" {
		b.fail("hazirlayan", "required")
	}
	if b.donemStart == "" || b.donemEnd == "" {
		b.fail("period", "required")
	}
	if b.bolumStart == "" || b.bolumEnd == "" || b.bolumNo == "" {
		b.fail("section", "required")
	}
	if b.signatureInner == "" {
		b.fail("signature", "required")
	}
	if len(b.invoices) == 0 {
		b.fail("fatura", "required")
	}
	if b.err != nil {
		return nil, b.err
	}

	raporNo := b.raporNo
	if raporNo == "" {
		id, err := newUUID()
		if err != nil {
			return nil, builderWrap("raporNo", err)
		}
		raporNo = id
	}

	rep := &earchive.EArsivRaporu{
		Baslik: earchive.BaslikType{
			Versiyon:             b.version,
			Mukellef:             vknTckn(b.mukellefVKN, b.mukellefTCKN),
			Hazirlayan:           vknTckn(b.hazirlayanVKN, b.hazirlayanTCKN),
			RaporNo:              raporNo,
			DonemBaslangicTarihi: b.donemStart,
			DonemBitisTarihi:     b.donemEnd,
			BolumBaslangicTarihi: b.bolumStart,
			BolumBitisTarihi:     b.bolumEnd,
			BolumNo:              b.bolumNo,
			Signature:            earchive.XMLSignature{InnerXML: b.signatureInner},
		},
		Fatura: append([]earchive.Fatura{}, b.invoices...),
	}
	return rep, nil
}

func vknTckn(vkn, tckn string) earchive.VknTcknType {
	return earchive.VknTcknType{Vkn: vkn, Tckn: tckn}
}
