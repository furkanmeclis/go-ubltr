package qnb

import "encoding/xml"

// The services are JAX-WS document/literal with unqualified child
// elements: the operation element is in the service namespace and its
// children are in no namespace. Each request binds the "ser" prefix and
// resets the default namespace so encoding/xml emits exactly that.
type nsAttrs struct {
	Ser string `xml:"xmlns:ser,attr"`
	Def string `xml:"xmlns,attr"`
}

func connNS() nsAttrs { return nsAttrs{Ser: nsConnector} }
func arcNS() nsAttrs  { return nsAttrs{Ser: nsEArchive} }

// connectorService?xsd=1: belgeGonderExt(parametreler gidenBelgeParametreleri).
type belgeGonderExtReq struct {
	XMLName xml.Name `xml:"ser:belgeGonderExt"`
	nsAttrs
	Params gidenBelgeParametreleri `xml:"parametreler"`
}

type gidenBelgeParametreleri struct {
	AlanEtiket      string `xml:"alanEtiket,omitempty"`
	BelgeHash       string `xml:"belgeHash"`
	BelgeNo         string `xml:"belgeNo"`
	BelgeTuru       string `xml:"belgeTuru"`
	BelgeVersiyon   string `xml:"belgeVersiyon"`
	ErpKodu         string `xml:"erpKodu,omitempty"`
	GonderenEtiket  string `xml:"gonderenEtiket,omitempty"`
	MimeType        string `xml:"mimeType"`
	SeriNo          string `xml:"seriNo,omitempty"`
	VergiTcKimlikNo string `xml:"vergiTcKimlikNo"`
	Veri            string `xml:"veri"`
}

type belgeGonderExtResp struct {
	BelgeOid string `xml:"belgeOid"`
}

// gidenBelgeDurumSorgulaExt(vergiTcKimlikNo, parametreler gidenBelgeDurumParametreleri).
type gidenBelgeDurumSorgulaExtReq struct {
	XMLName xml.Name `xml:"ser:gidenBelgeDurumSorgulaExt"`
	nsAttrs
	VKN    string                       `xml:"vergiTcKimlikNo"`
	Params gidenBelgeDurumParametreleri `xml:"parametreler"`
}

type gidenBelgeDurumParametreleri struct {
	BelgeNo           string `xml:"belgeNo"`
	BelgeNoTipi       string `xml:"belgeNoTipi"`
	BelgeTuru         string `xml:"belgeTuru"`
	DonusTipiVersiyon string `xml:"donusTipiVersiyon"`
}

type gidenBelgeDurumSorgulaExtResp struct {
	Return gidenBelgeDurum `xml:"return"`
}

// gidenBelgeDurum and its v2..v7 extensions (xsi:type), flattened.
type gidenBelgeDurum struct {
	Aciklama                string `xml:"aciklama"`
	AlimTarihi              string `xml:"alimTarihi"`
	BelgeNo                 string `xml:"belgeNo"`
	Durum                   int    `xml:"durum"`
	Ettn                    string `xml:"ettn"`
	GonderimCevabiDetayi    string `xml:"gonderimCevabiDetayi"`
	GonderimCevabiKodu      int    `xml:"gonderimCevabiKodu"`
	GonderimDurumu          int    `xml:"gonderimDurumu"`
	GonderimTarihi          string `xml:"gonderimTarihi"`
	OlusturulmaTarihi       string `xml:"olusturulmaTarihi"`
	YanitDetayi             string `xml:"yanitDetayi"`
	YanitDurumu             int    `xml:"yanitDurumu"`
	YanitTarihi             string `xml:"yanitTarihi"`
	UlastiMi                bool   `xml:"ulastiMi"`
	YenidenGonderilebilirMi bool   `xml:"yenidenGonderilebilirMi"`
	YerelBelgeOid           string `xml:"yerelBelgeOid"`
}

// gelenBelgeleriListeleExt / gelenBelgeleriIndirExt(parametreler gelenBelgeParametreleri).
type gelenBelgeleriListeleExtReq struct {
	XMLName xml.Name `xml:"ser:gelenBelgeleriListeleExt"`
	nsAttrs
	Params gelenBelgeParametreleri `xml:"parametreler"`
}

type gelenBelgeleriIndirExtReq struct {
	XMLName xml.Name `xml:"ser:gelenBelgeleriIndirExt"`
	nsAttrs
	Params gelenBelgeParametreleri `xml:"parametreler"`
}

type gelenBelgeParametreleri struct {
	BelgeFormati               string   `xml:"belgeFormati,omitempty"`
	BelgeTuru                  string   `xml:"belgeTuru"`
	BelgelerAlindiMi           *bool    `xml:"belgelerAlindiMi,omitempty"`
	DonusTipiVersiyon          string   `xml:"donusTipiVersiyon,omitempty"`
	ErpKodu                    string   `xml:"erpKodu,omitempty"`
	Ettn                       []string `xml:"ettn,omitempty"`
	SonAlinanBelgeSiraNumarasi string   `xml:"sonAlinanBelgeSiraNumarasi,omitempty"`
	VergiTcKimlikNo            string   `xml:"vergiTcKimlikNo"`
}

type gelenBelgeleriListeleExtResp struct {
	Return []belge `xml:"return"`
}

// belge and its v2..v7 extensions, flattened.
type belge struct {
	AliciVkn                string `xml:"aliciVkn"`
	BelgeNo                 string `xml:"belgeNo"`
	BelgeSiraNo             string `xml:"belgeSiraNo"`
	BelgeTarihi             string `xml:"belgeTarihi"`
	BelgeTuru               string `xml:"belgeTuru"`
	DovizCinsi              string `xml:"dovizCinsi"`
	Ettn                    string `xml:"ettn"`
	FaturaSenaryo           string `xml:"faturaSenaryo"`
	GonderenEtiket          string `xml:"gonderenEtiket"`
	GonderenIsim            string `xml:"gonderenIsim"`
	GonderenVknTckn         string `xml:"gonderenVknTckn"`
	PayableAmount           string `xml:"payableAmount"`
	AlanEtiket              string `xml:"alanEtiket"`
	AliciUnvan              string `xml:"aliciUnvan"`
	SaticiUnvan             string `xml:"saticiUnvan"`
	ZarfID                  string `xml:"zarfId"`
	OdenecekTutar           string `xml:"odenecekTutar"`
	OdenecekTutarDovizCinsi string `xml:"odenecekTutarDovizCinsi"`
	FaturaGelisTarihi       string `xml:"faturaGelisTarihi"`
	ProfileID               string `xml:"profileId"`
}

type base64Resp struct {
	Return string `xml:"return"`
}

// gidenBelgeleriIndir(vergiTcKimlikNo, belgeOidListesi*, belgeTuru, belgeFormati).
type gidenBelgeleriIndirReq struct {
	XMLName xml.Name `xml:"ser:gidenBelgeleriIndir"`
	nsAttrs
	VKN       string   `xml:"vergiTcKimlikNo"`
	OIDs      []string `xml:"belgeOidListesi"`
	BelgeTuru string   `xml:"belgeTuru"`
	Format    string   `xml:"belgeFormati"`
}

// belgelerAlindi(vergiTcKimlikNo, ettn*, belgeTuru).
type belgelerAlindiReq struct {
	XMLName xml.Name `xml:"ser:belgelerAlindi"`
	nsAttrs
	VKN       string   `xml:"vergiTcKimlikNo"`
	Ettn      []string `xml:"ettn"`
	BelgeTuru string   `xml:"belgeTuru"`
}

// efaturaKullaniciBilgisi(vergiTcKimlikNo) -> eFaturaKullanici*.
type efaturaKullaniciBilgisiReq struct {
	XMLName xml.Name `xml:"ser:efaturaKullaniciBilgisi"`
	nsAttrs
	VKN string `xml:"vergiTcKimlikNo"`
}

type efaturaKullaniciBilgisiResp struct {
	Return []eFaturaKullanici `xml:"return"`
}

type eFaturaKullanici struct {
	Etiket          string `xml:"etiket"`
	KamuKurulusu    bool   `xml:"kamuKurulusu"`
	KayitZamani     string `xml:"kayitZamani"`
	Unvan           string `xml:"unvan"`
	VergiTcKimlikNo string `xml:"vergiTcKimlikNo"`
}

// kontorBilgisiGetir(vknTckn, kontorTipi, kontorBirimi) -> kalanKontorBilgisi.
type kontorBilgisiGetirReq struct {
	XMLName xml.Name `xml:"ser:kontorBilgisiGetir"`
	nsAttrs
	VKN    string `xml:"vknTckn"`
	Tip    string `xml:"kontorTipi"`
	Birimi string `xml:"kontorBirimi"`
}

type kontorBilgisiGetirResp struct {
	Return struct {
		Kalan        string `xml:"kalan"`
		ToplamAlinan string `xml:"toplamAlinan"`
		KontorTipi   string `xml:"kontorTipi"`
		KontorBirimi string `xml:"kontorBirimi"`
	} `xml:"return"`
}

// EarsivWebService?xsd=1: faturaOlusturExt(input, fatura belge) ->
// belgeOutputWrapper; faturaSorgulaExt(input) -> belgeOutputWrapper;
// faturaIptalEt(input) -> earsivServiceResult.
type faturaOlusturExtReq struct {
	XMLName xml.Name `xml:"ser:faturaOlusturExt"`
	nsAttrs
	Input  string      `xml:"input"`
	Fatura earsivBelge `xml:"fatura"`
}

type earsivBelge struct {
	BelgeFormati string `xml:"belgeFormati"`
	BelgeIcerigi string `xml:"belgeIcerigi"`
}

type faturaSorgulaExtReq struct {
	XMLName xml.Name `xml:"ser:faturaSorgulaExt"`
	nsAttrs
	Input string `xml:"input"`
}

type faturaIptalEtReq struct {
	XMLName xml.Name `xml:"ser:faturaIptalEt"`
	nsAttrs
	Input string `xml:"input"`
}

type belgeOutputWrapper struct {
	Output earsivBelge         `xml:"output"`
	Return earsivServiceResult `xml:"return"`
}

type faturaIptalEtResp struct {
	Return earsivServiceResult `xml:"return"`
}

type earsivServiceResult struct {
	ResultCode string `xml:"resultCode"`
	ResultText string `xml:"resultText"`
	Extra      []struct {
		Key   string `xml:"key"`
		Value string `xml:"value"`
	} `xml:"resultExtra>entry"`
}

func (r earsivServiceResult) extra(key string) string {
	for _, e := range r.Extra {
		if e.Key == key {
			return e.Value
		}
	}
	return ""
}
