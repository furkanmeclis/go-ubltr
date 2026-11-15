package izibiz

import (
	"strconv"
	"strings"

	"github.com/furkanmeclis/go-ubltr/provider"
)

// invoiceStatusCodes maps EInvoiceWS / EIrsaliyeWS STATUS_CODE values to a
// normalized status. Source: İzibiz Developer Portal, "Giden Fatura
// Durumları", "Gelen Fatura Durumları" and "E-İrsaliye Durumları".
// 134 (ZAMAN AŞIMI) is retried by the integrator, so it stays processing.
var invoiceStatusCodes = map[int]provider.Status{
	100: provider.StatusQueued,     // DURUM HENÜZ GÜNCELLENMEDİ
	101: provider.StatusQueued,     // KUYRUĞA EKLENDİ
	102: provider.StatusProcessing, // TASLAK İŞLENİYOR
	103: provider.StatusProcessing, // PAKETLENİYOR
	104: provider.StatusProcessing, // PAKETLENDİ
	105: provider.StatusProcessing, // PAKETLEME HATASI, tekrar denenecek
	106: provider.StatusProcessing, // İMZALANIYOR
	107: provider.StatusProcessing, // İMZALANDI
	109: provider.StatusProcessing, // GİB'DEN YANIT BEKLİYOR
	110: provider.StatusSent,       // ALICIDAN YANIT BEKLİYOR
	111: provider.StatusDelivered,  // ALICIDAN ONAY BEKLİYOR
	112: provider.StatusAccepted,   // KABUL EDİLDİ
	117: provider.StatusRejected,   // RET EDİLDİ, alıcıdan yanıt bekliyor
	120: provider.StatusRejected,   // RET EDİLDİ
	122: provider.StatusAccepted,   // gelen: KABUL EDİLDİ
	123: provider.StatusProcessing, // gelen: KABUL İŞLENİYOR
	124: provider.StatusProcessing, // gelen: KABUL GİB'DEN YANIT BEKLİYOR
	125: provider.StatusProcessing, // gelen: KABUL ALICIDAN YANIT BEKLİYOR
	126: provider.StatusFailed,     // gelen: KABUL İŞLEMİ BAŞARISIZ
	127: provider.StatusProcessing, // gelen: RED ALICIDAN YANIT BEKLİYOR
	128: provider.StatusProcessing, // gelen: RED GİB'DE YANIT BEKLİYOR
	129: provider.StatusProcessing, // gelen: RED İŞLENİYOR
	130: provider.StatusRejected,   // gelen: REDDEDİLDİ
	131: provider.StatusFailed,     // gelen: RED İŞLEMİ BAŞARISIZ
	132: provider.StatusReceived,   // gelen: ALINDI, yanıt bekliyor
	133: provider.StatusReceived,   // gelen: ALINDI
	134: provider.StatusProcessing, // ZAMAN AŞIMI, tekrar denenecek
	135: provider.StatusProcessing, // GÖNDERİLİYOR
	136: provider.StatusFailed,     // GÖNDERME İŞLEMİ BAŞARISIZ
	137: provider.StatusDelivered,  // GÖNDERİLDİ
	139: provider.StatusFailed,     // OTOMATİK GÖNDERİM HATASI
	140: provider.StatusQueued,     // FATURA NUMARASI ATANDI
	141: provider.StatusQueued,     // FATURA NUMARASI ATANMA BEKLENİYOR
}

// statusWords maps the "STATUS - SUBSTATUS" text (spaces removed) when no
// numeric code is present. Values come from the same portal tables and
// from the ws-client-dotnet sample (EI.StatusType / EI.SubStatusType).
var statusWords = map[string]provider.Status{
	"LOAD-SUCCEED":                      provider.StatusQueued,
	"LOAD-PROCESSING":                   provider.StatusQueued,
	"LOAD-FAILED":                       provider.StatusFailed,
	"PACKAGE-PROCESSING":                provider.StatusProcessing,
	"PACKAGE-SUCCEED":                   provider.StatusProcessing,
	"PACKAGE-FAILED":                    provider.StatusProcessing,
	"SIGN-PROCESSING":                   provider.StatusProcessing,
	"SIGN-SUCCEED":                      provider.StatusProcessing,
	"SIGN-FAILED":                       provider.StatusProcessing,
	"SEND-PROCESSING":                   provider.StatusProcessing,
	"SEND-WAIT_GIB_RESPONSE":            provider.StatusProcessing,
	"SEND-WAIT_SYSTEM_RESPONSE":         provider.StatusSent,
	"SEND-WAIT_APPLICATION_RESPONSE":    provider.StatusDelivered,
	"SEND-SUCCEED":                      provider.StatusDelivered,
	"SEND-FAILED":                       provider.StatusFailed,
	"SEND-TIMEOUT":                      provider.StatusProcessing,
	"AUTO-FAILED":                       provider.StatusFailed,
	"ACCEPTED-SUCCEED":                  provider.StatusAccepted,
	"REJECTED-SUCCEED":                  provider.StatusRejected,
	"REJECTED-WAIT_SYSTEM_RESPONSE":     provider.StatusRejected,
	"RECEIVE-SUCCEED":                   provider.StatusReceived,
	"RECEIVE-WAIT_APPLICATION_RESPONSE": provider.StatusReceived,
	"ACCEPT-PROCESSING":                 provider.StatusProcessing,
	"ACCEPT-WAIT_GIB_RESPONSE":          provider.StatusProcessing,
	"ACCEPT-WAIT_SYSTEM_RESPONSE":       provider.StatusProcessing,
	"ACCEPT-SUCCEED":                    provider.StatusAccepted,
	"ACCEPT-FAILED":                     provider.StatusFailed,
	"REJECT-PROCESSING":                 provider.StatusProcessing,
	"REJECT-WAIT_GIB_RESPONSE":          provider.StatusProcessing,
	"REJECT-WAIT_SYSTEM_RESPONSE":       provider.StatusProcessing,
	"REJECT-SUCCEED":                    provider.StatusRejected,
	"REJECT-FAILED":                     provider.StatusFailed,
}

// archiveStatusCodes maps EIArchiveWS STATUS values ("E-Arşiv Fatura
// Durumları"). An e-Arşiv invoice is final once reported (130).
var archiveStatusCodes = map[int]provider.Status{
	100: provider.StatusQueued,     // KUYRUĞA EKLENDİ
	105: provider.StatusDraft,      // TASLAK OLARAK EKLENDİ
	110: provider.StatusProcessing, // İŞLENİYOR
	120: provider.StatusSent,       // RAPORLANACAK
	130: provider.StatusDelivered,  // RAPORLANDI
	150: provider.StatusCancelled,  // RAPORLANMADAN İPTAL EDİLDİ
	200: provider.StatusFailed,     // FATURA ID BULUNAMADI
}

// documentStatus combines STATUS_CODE, STATUS text and GIB_STATUS_CODE. A
// failing GİB code wins over an in-progress vendor status.
func documentStatus(statusCode, status string, gibCode int) provider.Status {
	s := provider.StatusUnknown
	if n, err := strconv.Atoi(strings.TrimSpace(statusCode)); err == nil {
		if v, ok := invoiceStatusCodes[n]; ok {
			s = v
		}
	}
	if s == provider.StatusUnknown {
		if v, ok := statusWords[normalizeWords(status)]; ok {
			s = v
		}
	}
	g := provider.GIBStatus(gibCode)
	switch {
	case s == provider.StatusUnknown:
		s = g
	case g == provider.StatusFailed && !s.Terminal():
		s = g
	}
	return s
}

func normalizeWords(s string) string {
	return strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(s), " ", ""))
}

// responseCode returns KABUL / RED for a commercial outcome.
func responseCode(s provider.Status) string {
	switch s {
	case provider.StatusAccepted:
		return "KABUL"
	case provider.StatusRejected:
		return "RED"
	}
	return ""
}

func archiveStatus(code string) provider.Status {
	if n, err := strconv.Atoi(strings.TrimSpace(code)); err == nil {
		if v, ok := archiveStatusCodes[n]; ok {
			return v
		}
	}
	return provider.StatusUnknown
}
