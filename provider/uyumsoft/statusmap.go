package uyumsoft

import (
	"strconv"

	"github.com/furkanmeclis/go-ubltr/provider"
)

// statusInput is what a status mapping can know about a document.
type statusInput struct {
	Raw       string // InvoiceStatus text
	Code      int    // StatusCode
	Direction provider.Direction
	// Commercial is true when the document is known to be TICARIFATURA
	// (list rows carry Type/Profile; status queries do not).
	Commercial bool
}

// mapStatus maps an Uyumsoft InvoiceStatus onto provider.Status.
//
// The base mapping is the "status:<name>" rows of errors.csv. Adjustments:
//   - Inbound documents are StatusReceived unless they were answered;
//     the inbox Status is the application-response state (Approved,
//     WaitingForAprovement, Declined).
//   - Approved is StatusAccepted only for a known TICARIFATURA; for other
//     profiles and e-Arşiv it means "completed" (StatusDelivered).
//   - When the text is unknown and the preset documents its numeric
//     StatusCode values, those are used.
func mapStatus(p Preset, in statusInput) provider.Status {
	st := provider.StatusUnknown
	if info, ok := Codes.Lookup("status:" + in.Raw); ok && in.Raw != "" {
		st = info.Status
	} else if p.NumericStatus {
		st = numericStatus(in.Code)
	}
	switch in.Raw {
	case "Approved":
		if in.Commercial {
			st = provider.StatusAccepted
		} else if in.Direction == provider.Inbound {
			st = provider.StatusReceived
		}
	case "WaitingForAprovement":
		if in.Direction == provider.Inbound {
			st = provider.StatusReceived
		}
	}
	return st
}

// responseCode returns KABUL / RED / IADE for an answered commercial
// invoice. An inbound Declined or Return can only be an application
// response, so it does not need the profile.
func responseCode(in statusInput) string {
	if !in.Commercial && (in.Direction != provider.Inbound || in.Raw == "Approved") {
		return ""
	}
	switch in.Raw {
	case "Approved":
		return "KABUL"
	case "Declined":
		return "RED"
	case "Return":
		return "IADE"
	}
	return ""
}

// numericStatus maps the StatusCode scale muhasebeizleme documents
// (Mi API Kılavuzu, "Durum ve Kod Tabloları", KODLAR.giden): 0 Draft,
// 10 Canceled (draft deleted), 100 Queued, 200 Processing, 300 SentToGib,
// 1000 Approved, 1100 WaitingForAprovement, 1200 Declined, 1400 Canceled,
// 2000 Error. Unknown codes are in flight; they stay StatusUnknown.
func numericStatus(code int) provider.Status {
	switch code {
	case 0:
		return provider.StatusDraft
	case 10, 1400:
		return provider.StatusCancelled
	case 100:
		return provider.StatusQueued
	case 200:
		return provider.StatusProcessing
	case 300:
		return provider.StatusSent
	case 1000, 1100:
		return provider.StatusDelivered
	case 1200:
		return provider.StatusRejected
	case 2000:
		return provider.StatusFailed
	}
	return provider.StatusUnknown
}

// envelopeCodes maps the WSDL EnvelopeStatus enum names onto GİB envelope
// codes, as tabulated in the Mi API Kılavuzu (KODLAR.zarf). NoEnvelope,
// Preparing and DoesNotHaveEntegratorApplication have no GİB code.
var envelopeCodes = map[string]int{
	"EnvelopIsQueued":                   1000,
	"EnvelopIsProcessing":               1100,
	"FileIsNotZip":                      1110,
	"InvalidEnvelopIdLength":            1111,
	"EnvelopCouldNotCopiedFromArchive":  1120,
	"CouldNotOpenZip":                   1130,
	"ZipIsEmpty":                        1131,
	"FileIsNotXml":                      1132,
	"EnvelopeIdAndXmlNameMustBeSame":    1133,
	"CouldNotParseDocument":             1140,
	"EnvelopeIdNotFound":                1141,
	"EnvelopeIdAndZipNameMustBeSame":    1142,
	"InvalidVersion":                    1143,
	"SchematronCheckFailed":             1150,
	"XmlSchemaCheckFailed":              1160,
	"CouldNotTakeTcknVknForSigner":      1161,
	"CouldNotSaveSigniture":             1162,
	"EnvelopeIdIsAlreadyUsed":           1163,
	"EnvelopeContainsIdIsAlreadyUsed":   1164,
	"CouldNotCheckPermission":           1170,
	"DoesNotHaveSenderUnitPermission":   1171,
	"DoesNotHavePostBoxPermission":      1172,
	"CouldNotCheckSignPermission":       1175,
	"SignerHasNoPermission":             1176,
	"IllegalSign":                       1177,
	"CouldNotCheckAddress":              1180,
	"AddressNotFound":                   1181,
	"CouldNotPrepareSystemResponse":     1190,
	"SystemError":                       1195,
	"EnvelopedProcessSuccessfully":      1200,
	"CouldNotSendDocumentToTheAddress":  1210,
	"DocumentSendingFailedWillNotRetry": 1215,
	"TargetDoesNotSendSystemResponse":   1220,
	"TargetSendFailedSystemResponse":    1230,
	"InvoiceLinkedToCancel":             1235,
	"CompletedSuccessfully":             1300,
	"CouldNotFindEnvelopeId":            2004,
}

// gibCode returns the GİB envelope code for an EnvelopeStatus name, or
// the numeric EnvelopeStatusCode when it is a known GİB code.
func gibCode(name string, code int) int {
	if n, ok := envelopeCodes[name]; ok {
		return n
	}
	if _, ok := provider.GIBCodes.Lookup(strconv.Itoa(code)); ok {
		return code
	}
	return 0
}

// profiles maps the WSDL InvoiceTypes enum onto UBL ProfileID (Mi API
// Kılavuzu, KODLAR.senaryo).
var profiles = map[string]string{
	"BaseInvoice":               "TEMELFATURA",
	"ComercialInvoice":          "TICARIFATURA",
	"InvoiceWithPassanger":      "YOLCUBERABERFATURA",
	"Export":                    "IHRACAT",
	"eArchive":                  "EARSIVFATURA",
	"Hks":                       "HKS",
	"PublicAdministration":      "KAMU",
	"Energy":                    "ENERJI",
	"MedicineAndMedicalDevices": "ILAC_TIBBICIHAZ",
	"InvestmentIncentive":       "YATIRIMTESVIK",
	"IdisSystem":                "IDIS",
}

// invoiceTypes maps the WSDL InvoiceTipType enum onto UBL InvoiceTypeCode
// (Mi API Kılavuzu, KODLAR.faturaTipi).
var invoiceTypes = map[string]string{
	"Sales":                               "SATIS",
	"Return":                              "IADE",
	"Tax":                                 "TEVKIFAT",
	"Exception":                           "ISTISNA",
	"TaxBase":                             "OZELMATRAH",
	"ExportSaved":                         "IHRACKAYITLI",
	"Sgk":                                 "SGK",
	"Broker":                              "KOMISYONCU",
	"HksSales":                            "HKSSATIS",
	"HksBroker":                           "HKSKOMISYONCU",
	"WithholdingReturn":                   "TEVKIFATIADE",
	"Accomodation":                        "KONAKLAMAVERGISI",
	"Charge":                              "SARJ",
	"ChargeInstant":                       "SARJANLIK",
	"TechSupport":                         "TEKNOLOJIDESTEK",
	"InvestmentIncentiveSales":            "YTBSATIS",
	"InvestmentIncentiveException":        "YTBISTISNA",
	"InvestmentIncentiveReturn":           "YTBIADE",
	"InvestmentIncentiveWitholding":       "YTBTEVKIFAT",
	"InvestmentIncentiveWitholdingReturn": "YTBTEVKIFATIADE",
}
