package uyumsoft

import (
	_ "embed"
	"errors"
	"strings"
	"unicode"

	"github.com/furkanmeclis/go-ubltr/provider"
	"github.com/furkanmeclis/go-ubltr/provider/soap"
)

//go:embed errors.csv
var errorsCSV string

// Codes is the embedded code table (errors.csv).
var Codes = provider.MustParseCodeTable("uyumsoft", errorsCSV)

// Message classes (errors.csv rows "message:<class>").
const (
	classAuth           = "auth"
	classQuota          = "quota"
	classPermission     = "permission"
	classDuplicate      = "duplicate"
	classInvalidState   = "invalid-state"
	classRecipient      = "recipient"
	classInvalidRequest = "invalid-request"
	classValidation     = "validation"
	classNotFound       = "not-found"
	classUnavailable    = "unavailable"
)

// messageRules classify message-only failures. Rules run in order and
// match folded text (see fold). Keywords come from the messages
// muhasebeizleme documents per operation and from generic wording; the
// order resolves overlaps such as "Kontör bilgisi alınamadı" (transient,
// not quota) or "PK etiketi bulunamadı" (recipient, not not-found).
var messageRules = []struct {
	class    string
	keywords []string
}{
	{classAuth, []string{
		"apikey", "api key", "kullanici adi", "sifre", "parola", "password",
		"authenticat", "unauthori", "kimlik dogrulama", "oturum", "verifying security",
		"security token", "credential",
	}},
	{classQuota, []string{
		"kontor yetersiz", "yeterli kontor", "kontorunuz", "kontor bulunma",
		"bakiye yetersiz", "yetersiz bakiye", "insufficient credit",
	}},
	{classPermission, []string{
		"modulunuz kapali", "modulu kapali", "baska bir mukellef adina", "yetkiniz yok",
		"yetkisi yok", "not authorized", "permission",
	}},
	{classDuplicate, []string{
		"daha once gonderil", "daha once kullanil", "daha once kayit", "zaten kayitli",
		"mukerrer", "duplicate", "already exist", "already used", "already registered",
	}},
	{classInvalidState, []string{
		"iptal suresi", "zaten iptal", "cevaplanamaz", "cevaplanmistir", "cevap verilemez",
		"yanit verilmesi gerekmektedir", "already cancel",
	}},
	{classRecipient, []string{
		"mukellefi degil", "mukellefi olmadigi", "kayitli kullanici degil", "posta kutusu",
		"pk etiketi", "etiketi aliciya", "alias", "not an e-invoice user",
	}},
	{classInvalidRequest, []string{
		"bos olamaz", "zorunludur", "desteklenmeyen", "gecersiz parametre", "is required",
		"cannot be empty", "invalid parameter",
	}},
	{classValidation, []string{
		"ubl icinde", "sema", "schema", "schematron", "xsd", "fatura tarihi", "para birim",
		"invoicetypecode", "profileid", "belge serisi", "validation", "dogrulama",
		"gecersiz", "hatali", "invalid",
	}},
	{classNotFound, []string{"bulunamadi", "bulunamamistir", "not found", "bulunmamaktadir"}},
	{classUnavailable, []string{
		"alinamadi", "yapilamadi", "degistirilemedi", "cozumlenemedi", "ulasilamadi",
		"tekrar deneyin", "gecici", "timeout", "unavailable", "zaman asimi",
	}},
}

// classifyMessage returns the message class for a vendor message, or "".
func classifyMessage(msg string) string {
	f := fold(msg)
	if f == "" {
		return ""
	}
	for _, r := range messageRules {
		for _, k := range r.keywords {
			if strings.Contains(f, k) {
				return r.class
			}
		}
	}
	return ""
}

// fold lowercases with Turkish rules and strips Turkish diacritics so
// keywords can be written in ASCII ("kontör" → "kontor", "İptal" → "iptal").
func fold(s string) string {
	s = strings.ToLowerSpecial(unicode.TurkishCase, s)
	return strings.NewReplacer(
		"ı", "i", "i̇", "i", "ğ", "g", "ü", "u", "ş", "s", "ö", "o", "ç", "c", "â", "a", "î", "i", "û", "u",
	).Replace(s)
}

// messageError builds an *provider.Error for an IsSucceded="false" (or
// Value="false") response. Code stays empty: the vendors send no codes.
func messageError(name, op, msg string) *provider.Error {
	class := classifyMessage(msg)
	if class == "" {
		return &provider.Error{Kind: provider.KindUnknown, Provider: name, Op: op, Message: msg}
	}
	e := Codes.NewError(name, op, "message:"+class, msg)
	e.Code = ""
	return e
}

// faultError reclassifies a SOAP fault from soap.Client using the fault
// code table and the message rules. Other errors pass through.
func (c *Client) faultError(op string, err error) error {
	var pe *provider.Error
	if !errors.As(err, &pe) {
		return err
	}
	var f *soap.Fault
	if !errors.As(pe, &f) {
		return err
	}
	local := f.Code
	if i := strings.LastIndexByte(local, ':'); i >= 0 {
		local = local[i+1:]
	}
	if i := strings.LastIndexByte(local, '/'); i >= 0 {
		// SOAP 1.2 "Code/Subcode": the subcode is more specific.
		local = strings.TrimPrefix(local[i+1:], "a:")
	}
	kind, retry := pe.Kind, pe.Retryable
	if info, ok := Codes.Lookup("fault:" + local); ok {
		kind, retry = info.Kind, info.Retryable
	} else if class := classifyMessage(f.Reason); class != "" {
		info, _ := Codes.Lookup("message:" + class)
		kind, retry = info.Kind, info.Retryable
	} else if pe.HTTPStatus >= 500 && kind == provider.KindTransport {
		// WCF answers every fault with HTTP 500; an unclassified fault is
		// not evidence of an outage, so it is not marked retryable.
		retry = false
	}
	out := *pe
	out.Kind, out.Retryable, out.Op = kind, retry, op
	return &out
}
