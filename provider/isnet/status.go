package isnet

import (
	"strconv"
	"strings"

	"github.com/furkanmeclis/go-ubltr/provider"
)

// detailCodes maps InvoiceDetailStatus names to their GİB envelope codes,
// as enumerated in the InvoiceService WSDL (EnumerationValue).
var detailCodes = map[string]int{
	"Zarflanmadi":                                          1,
	"Zarf_Kuyruga_Eklendi":                                 1000,
	"Zarf_Isleniyor":                                       1100,
	"Zip_Dosyasi_Degil":                                    1110,
	"ZarfId_Uzunlugu_Gecersiz":                             1111,
	"Zarf_Arsivden_Kopyalanamadi":                          1120,
	"Zip_Acilamadi":                                        1130,
	"Zip_Bir_Dosya_Icermeli":                               1131,
	"XML_Dosyasi_Degil":                                    1132,
	"Zarf_ID_Ve_XML_Dosyasinin_Adi_Ayni_Olmali":            1133,
	"Dokuman_Ayristirilamadi":                              1140,
	"Zarf_ID_Yok":                                          1141,
	"Zarf_ID_VE_Zip_Dosyasi_Adi_Ayni_Olmali":               1142,
	"Gecersiz_Versiyon":                                    1143,
	"Schematron_Kontrol_Sonucu_Hatali":                     1150,
	"Xml_Sema_Kontrolundan_Gecemedi":                       1160,
	"Imza_Sahibi_TCKN_VKN_Alinamadi":                       1161,
	"Imza_Kaydedilemedi":                                   1162,
	"Gonderilen_Zarf_Kayitli_bir_Fatura_Icermelidir":       1163,
	"Gonderilen_Zarf_Kayitli_bir_Belge_Icermektedir":       1164,
	"Yetki_Kontrol_Edilemedi":                              1170,
	"Gonderici_Birim_Yetkisi_Yok":                          1171,
	"Posta_Kutusu_Yetkisi_Yok":                             1172,
	"Islem_Yetkisi_Yok":                                    1173,
	"Fatura_Islem_Yetkisi_Yok":                             1174,
	"Imza_Yetkisi_Kontrol_Edilemedi":                       1175,
	"Imza_Sahibi_Yetkisi":                                  1176,
	"Gecersiz_Imza":                                        1177,
	"Adres_Kontrol_Edilemedi":                              1180,
	"Adres_Bulunamadi":                                     1181,
	"Kullanici_Eklenemedi":                                 1182,
	"Kullanici_Silinemedi":                                 1183,
	"Sistem_Yaniti_Hazirlanamadi":                          1190,
	"Sistem_Hatasi":                                        1195,
	"Zarf_Basariyla_Islendi":                               1200,
	"Dokuman_Bulunan_Adrese_Gonderilemedi":                 1210,
	"Dokuman_Gonderimi_Basarisiz_Tekrar_Gonderme_Sonlandi": 1215,
	"Hedeften_Sistem_Yaniti_Gelmedi":                       1220,
	"Hedeften_Sistem_Yaniti_Basarisiz_Geldi":               1230,
	"Fatura_Iptale_Konu_Edildi":                            1235,
	"Basariyla_Tamamlandi":                                 1300,
}

// invoiceStatusNames maps the numeric InvoiceStatus values (WSDL and
// integration note §8.1) to their names, for responses that send numbers.
var invoiceStatusNames = map[string]string{
	"0": "Onay_Bekliyor", "1": "Onaylandi", "2": "Reddedildi", "3": "Onay_Akisinda",
	"4": "Iade_Edildi", "8": "Gonderildi", "12": "Ziplendi", "13": "Gibe_Iletildi",
	"15": "Imza_Bekliyor", "16": "Gib_Tarafinda_Hata_Olustu", "22": "Sistem_Hatasi",
	"24": "Alici_Kabul_Etti", "25": "Alici_Reddetti", "26": "Alici_Iade_Etti",
	"29": "Otomatik_Onaylandi", "30": "Otomatik_Alici_Kabul_Etti",
	"36": "Uygulama_Yaniti_Yollaniyor", "37": "Uygulama_Yaniti_Hata_Aldi",
	"41": "Irsaliye_Yaniti_Yollaniyor", "42": "Irsaliye_Yaniti_Hata_Aldi",
}

// archiveStatusNames maps numeric ArchiveInvoiceStatus values.
var archiveStatusNames = map[string]string{
	"1": "Fatura_Hazirlaniyor", "2": "Fatura_Imzalaniyor", "3": "Pdf_Hazirlaniyor",
	"4": "Pdf_Imzalaniyor", "5": "Fatura_Olusturuldu", "6": "Silindi",
}

// detailCode returns the GİB code of an InvoiceDetailStatus value (name or
// number), or 0.
func detailCode(v string) int {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0
	}
	if n, err := strconv.Atoi(v); err == nil {
		return n
	}
	for name, n := range detailCodes {
		if strings.EqualFold(name, v) {
			return n
		}
	}
	return 0
}

// lookupState finds a prefixed Codes row, ignoring case.
func lookupState(prefix, v string, numeric map[string]string) (provider.CodeInfo, bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return provider.CodeInfo{}, false
	}
	if name, ok := numeric[v]; ok {
		v = name
	}
	if c, ok := Codes.Lookup(prefix + v); ok {
		return c, true
	}
	for _, c := range Codes.Codes() {
		if strings.EqualFold(c.Code, prefix+v) {
			return c, true
		}
	}
	return provider.CodeInfo{}, false
}

// einvoiceState is the normalized view of an e-Fatura Invoice item.
type einvoiceState struct {
	status   provider.Status
	gib      int
	raw      string // Status/DetailStatus as received
	response string // KABUL / RED / IADE when known
}

// mapInvoice maps Status (InvoiceStatus) and DetailStatus
// (InvoiceDetailStatus) of an e-Fatura item.
//
// Status decides when it names a definite outcome (approved, rejected,
// returned, error). For the neutral "sent" states, and for states that
// keep the previous status, the GİB code in DetailStatus decides through
// the shared GİB table (1300 delivered, 1200/1220 sent, 11xx failed, …).
// Incoming documents that are not accepted, rejected or failed are
// "received".
func mapInvoice(dir provider.Direction, status, detail string) einvoiceState {
	st := einvoiceState{status: provider.StatusUnknown, gib: detailCode(detail)}
	st.raw = strings.TrimSpace(status)
	if d := strings.TrimSpace(detail); d != "" {
		st.raw += "/" + d
	}
	name := strings.TrimSpace(status)
	if n, ok := invoiceStatusNames[name]; ok {
		name = n
	}
	info, known := lookupState("status.", name, nil)
	if known {
		st.status = info.Status
	}
	switch strings.ToLower(name) {
	case "onaylandi", "alici_kabul_etti", "otomatik_onaylandi", "otomatik_alici_kabul_etti":
		st.response = "KABUL"
	case "reddedildi", "alici_reddetti":
		st.response = "RED"
	case "iade_edildi", "alici_iade_etti":
		st.response = "IADE"
	}
	neutral := !known || st.status == provider.StatusUnknown || st.status == provider.StatusSent
	if neutral && st.gib >= 1000 {
		if g := provider.GIBStatus(st.gib); g != provider.StatusUnknown {
			st.status = g
		}
		if st.gib == 1235 { // Fatura_Iptale_Konu_Edildi
			st.status = provider.StatusCancelled
		}
	}
	if dir == provider.Inbound {
		switch st.status {
		case provider.StatusAccepted, provider.StatusRejected, provider.StatusFailed, provider.StatusCancelled:
		default:
			st.status = provider.StatusReceived
		}
	}
	return st
}

// archiveState is the normalized view of an e-Arşiv ArchiveInvoice item.
type archiveState struct {
	status provider.Status
	raw    string
	// reportError is set when ReportSendingStatus is a GİB report error.
	reportError string
}

// mapArchive maps Status (ArchiveInvoiceStatus), IsCanceled and
// CancellationReportSendingStatus of an e-Arşiv item. Any cancellation
// signal wins.
func mapArchive(status, isCanceled, cancelReport, report string) archiveState {
	st := archiveState{status: provider.StatusUnknown}
	var parts []string
	for _, p := range []string{status, cancelReport, report} {
		if p = strings.TrimSpace(p); p != "" {
			parts = append(parts, p)
		}
	}
	st.raw = strings.Join(parts, "/")
	if c, ok := lookupState("archive.", status, archiveStatusNames); ok {
		st.status = c.Status
	}
	if b, err := strconv.ParseBool(strings.TrimSpace(isCanceled)); err == nil && b {
		st.status = provider.StatusCancelled
	}
	if n, err := strconv.Atoi(strings.TrimSpace(cancelReport)); err == nil {
		if n != 0 { // 0 is Iptal_Edilmedi
			st.status = provider.StatusCancelled
		}
	} else if c, ok := lookupState("cancel.", cancelReport, nil); ok && c.Status == provider.StatusCancelled {
		st.status = provider.StatusCancelled
	}
	if c, ok := lookupState("report.", report, nil); ok && c.Kind != provider.KindOK {
		st.reportError = c.Message
	}
	return st
}
