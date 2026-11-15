package uyumsoft

import (
	"strings"
	"testing"

	"github.com/furkanmeclis/go-ubltr/provider"
)

func TestClassifyMessage(t *testing.T) {
	cases := []struct {
		msg  string
		want string
	}{
		// Messages documented per operation in the Mi API Kılavuzu.
		{"Geçersiz, süresi dolmuş veya yetkisiz ApiKey.", classAuth},
		{"Kontör yetersiz. Fatura gönderilemedi.", classQuota},
		{"Bu işlem Kontör tüketmektedir. Hesabınızda yeterli kontör bulunmadığı için işlem gerçekleştirilememiştir.", classQuota},
		{"Kontör bilgisi alınamadı.", classUnavailable},
		{"UBL içindeki gönderen VKN ('1234567890') hesabınızın VKN'si ile uyuşmuyor. Başka bir mükellef adına fatura gönderemezsiniz.", classPermission},
		{"e-Fatura modülünüz kapalı. Bu belge e-Fatura olarak kaydedilemez.", classPermission},
		{"Alıcı e-Fatura mükellefi olmadığı için bu belge e-Arşiv faturası olarak düzenlenmelidir, ancak e-Arşiv modülünüz kapalı.", classPermission},
		{"Alıcının e-Fatura posta kutusu (PK) etiketi bulunamadı.", classRecipient},
		{"Belirtilen posta kutusu etiketi alıcıya ait değil: 'urn:mail:x'.", classRecipient},
		{"UBL içinde AccountingCustomerParty/Party bulunamadı.", classValidation},
		{"Fatura tarihi 7 günden eski olamaz (VUK 231/5).", classValidation},
		{"Şu anda yalnız TRY para birimli faturalar kabul edilmektedir.", classValidation},
		{"ProfileID (senaryo) zorunludur.", classInvalidRequest},
		{"Tanınmayan InvoiceTypeCode: 'X'.", classValidation},
		{"İlgili belge tipine ait belge serisi (önek) tanımlaması yapılması gerekmektedir", classValidation},
		{"invoiceIds boş olamaz.", classInvalidRequest},
		{"VKN/TCKN zorunludur.", classInvalidRequest},
		{"Desteklenmeyen ResponseStatus (Approved/Declined bekleniyor).", classInvalidRequest},
		{"Belge bulunamadı.", classNotFound},
		{"Belge bulunamadı (e-Arşiv giden değil veya başka hesaba ait).", classNotFound},
		{"Belge zaten iptal edilmiş", classInvalidState},
		{"İptal süresi (belge tarihinden itibaren 8 gün) geçmiştir, sistem üzerinden iptal edilemez.", classInvalidState},
		{"İlgili Ticari Fatura daha önce cevaplanmıştır. Tekrar Cevaplanamaz", classInvalidState},
		{"İlgili Fatura Ticari Fatura Değildir Cevap Verilemez", classInvalidState},
		{"Uygulama yanıtı oluşturulamaz: Faturanın alındığı tarihten itibaren 8 gün içinde yanıt verilmesi gerekmektedir.", classInvalidState},
		{"Belge çözümlenemedi.", classUnavailable},
		{"Mükellef sorgusu yapılamadı.", classUnavailable},
		{"Giden kutusu listesi alınamadı.", classUnavailable},
		// Generic wording.
		{"An error occurred when verifying security for the message.", classAuth},
		{"Kullanıcı adı veya şifre hatalı", classAuth},
		{"Bu UUID ile daha önce gönderilmiş bir fatura bulunmaktadır", classDuplicate},
		{"Duplicate invoice UUID", classDuplicate},
		{"Alıcı e-fatura mükellefi değil", classRecipient},
		{"XML şema kontrolünden geçemedi", classValidation},
		{"", ""},
		{"Tamam", ""},
	}
	for _, tc := range cases {
		if got := classifyMessage(tc.msg); got != tc.want {
			t.Errorf("%q: %q, want %q", tc.msg, got, tc.want)
		}
	}
}

func TestMessageClassesInTable(t *testing.T) {
	for _, r := range messageRules {
		info, ok := Codes.Lookup("message:" + r.class)
		if !ok {
			t.Errorf("class %s has no errors.csv row", r.class)
			continue
		}
		if info.Kind == provider.KindOK || info.Kind == provider.KindUnknown {
			t.Errorf("class %s: kind %s", r.class, info.Kind)
		}
	}
	e := messageError("uyumsoft", "SendInvoice", "Kontör yetersiz.")
	if e.Kind != provider.KindQuotaExceeded || e.Code != "" || e.Message != "Kontör yetersiz." {
		t.Errorf("messageError = %+v", e)
	}
	if e := messageError("uyumsoft", "x", "bilinmeyen"); e.Kind != provider.KindUnknown {
		t.Errorf("unknown = %+v", e)
	}
}

func TestFold(t *testing.T) {
	if got := fold("İPTAL SÜRESİ Kontör ÇIKIŞ"); got != "iptal suresi kontor cikis" {
		t.Errorf("fold = %q", got)
	}
}

func TestEmbedInvoice(t *testing.T) {
	e, err := embedInvoice(readFile(t, "invoice.xml"))
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, a := range e.Attrs {
		names[a.Name.Local] = true
	}
	if !names["xmlns:cbc"] || !names["xmlns:cac"] || !names["xmlns:xsi"] || len(e.Attrs) != 3 {
		t.Errorf("attrs = %+v", e.Attrs)
	}
	if !strings.HasPrefix(string(e.Inner), "<cbc:UBLVersionID>") || !strings.HasSuffix(string(e.Inner), "</cac:AccountingCustomerParty>") {
		t.Errorf("inner = %s", e.Inner)
	}
	for _, bad := range []string{
		`<Invoice/>`,
		`<CreditNote xmlns="urn:oasis:names:specification:ubl:schema:xsd:CreditNote-2"/>`,
		`<Invoice xmlns="urn:x"/>`,
		``,
	} {
		if _, err := embedInvoice([]byte(bad)); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	e, err = embedInvoice([]byte(`<i:Invoice xmlns:i="urn:oasis:names:specification:ubl:schema:xsd:Invoice-2"/>`))
	if err != nil || len(e.Inner) != 0 {
		t.Errorf("self-closing = %+v %v", e, err)
	}
}
