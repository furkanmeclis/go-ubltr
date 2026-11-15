package izibiz

import (
	"encoding/xml"
	"strconv"
	"strings"
)

// Wire types for the İzibiz SOAP services. Only the elements used by this
// module are declared. Element names and order follow the published WSDL
// and XSD files kept under testdata/wsdl.
//
// Request roots are qualified with a prefix while their children stay
// unqualified, as the schemas declare no elementFormDefault (unqualified)
// and the vendor's own request samples show.

const (
	nsWSDL    = "http://schemas.i2i.com/ei/wsdl"         // AuthenticationWS, EInvoiceWS, EIrsaliyeWS
	nsArchive = "http://schemas.i2i.com/ei/wsdl/archive" // EIArchiveWS
)

// requestHeader is e:REQUEST_HEADERType. Field order matches the XSD.
type requestHeader struct {
	SessionID       string `xml:"SESSION_ID"`
	ClientTxnID     string `xml:"CLIENT_TXN_ID,omitempty"`
	ApplicationName string `xml:"APPLICATION_NAME,omitempty"`
	ChannelName     string `xml:"CHANNEL_NAME,omitempty"`
	Compressed      string `xml:"COMPRESSED,omitempty"`
}

// requestReturn is e:REQUEST_RETURNType. Numeric fields are kept as text so
// empty elements never break decoding.
type requestReturn struct {
	IntlTxnID  string `xml:"INTL_TXN_ID"`
	ReturnCode string `xml:"RETURN_CODE"`
}

// errorType is e:REQUEST_ERRORType, returned as ERROR_TYPE in a response
// body or inside a RequestFault detail.
type errorType struct {
	IntlTxnID string `xml:"INTL_TXN_ID"`
	Code      string `xml:"ERROR_CODE"`
	Short     string `xml:"ERROR_SHORT_DES"`
	Long      string `xml:"ERROR_LONG_DES"`
}

func (e *errorType) message() string {
	s := strings.TrimSpace(e.Short)
	if l := strings.TrimSpace(e.Long); l != "" && l != s {
		if s == "" {
			return l
		}
		s += " (" + l + ")"
	}
	return s
}

// result is embedded in every response that may carry ERROR_TYPE.
type result struct {
	Return *requestReturn `xml:"REQUEST_RETURN"`
	Error  *errorType     `xml:"ERROR_TYPE"`
}

func (r *result) errorType() *errorType { return r.Error }

// party is the SENDER / RECEIVER element with vkn and alias attributes.
type party struct {
	VKN   string `xml:"vkn,attr,omitempty"`
	Alias string `xml:"alias,attr,omitempty"`
}

// content is an xmime:base64Binary element.
type content struct {
	Value string `xml:",chardata"`
}

// Authentication.

type loginRequest struct {
	XMLName  xml.Name      `xml:"wsdl:LoginRequest"`
	NS       string        `xml:"xmlns:wsdl,attr"`
	Header   requestHeader `xml:"REQUEST_HEADER"`
	UserName string        `xml:"USER_NAME"`
	Password string        `xml:"PASSWORD"`
}

type loginResponse struct {
	result
	SessionID string `xml:"SESSION_ID"`
}

type checkUserRequest struct {
	XMLName      xml.Name      `xml:"wsdl:CheckUserRequest"`
	NS           string        `xml:"xmlns:wsdl,attr"`
	Header       requestHeader `xml:"REQUEST_HEADER"`
	User         gibUser       `xml:"USER"`
	DocumentType string        `xml:"DOCUMENT_TYPE,omitempty"`
}

type gibUser struct {
	Identifier   string `xml:"IDENTIFIER,omitempty"`
	Alias        string `xml:"ALIAS,omitempty"`
	Title        string `xml:"TITLE,omitempty"`
	Type         string `xml:"TYPE,omitempty"`
	RegisterTime string `xml:"REGISTER_TIME,omitempty"`
	Unit         string `xml:"UNIT,omitempty"`
	AliasCreated string `xml:"ALIAS_CREATION_TIME,omitempty"`
	Deleted      string `xml:"DELETED,omitempty"`
	DocumentType string `xml:"DOCUMENT_TYPE,omitempty"`
}

type checkUserResponse struct {
	result
	Users []gibUser `xml:"USER"`
}

// EInvoiceWS.

// invoiceRef is the INVOICE element used in requests.
type invoiceRef struct {
	ID      string   `xml:"ID,attr,omitempty"`
	UUID    string   `xml:"UUID,attr,omitempty"`
	Content *content `xml:"CONTENT,omitempty"`
}

type sendInvoiceRequest struct {
	XMLName  xml.Name      `xml:"wsdl:SendInvoiceRequest"`
	NS       string        `xml:"xmlns:wsdl,attr"`
	Header   requestHeader `xml:"REQUEST_HEADER"`
	Sender   *party        `xml:"SENDER,omitempty"`
	Receiver *party        `xml:"RECEIVER,omitempty"`
	XSLTName string        `xml:"XSLT_NAME,omitempty"`
	Prefix   string        `xml:"SERI_PREFIX,omitempty"`
	Invoices []invoiceRef  `xml:"INVOICE"`
}

type sendInvoiceResponse struct {
	result
	InvoiceID string `xml:"INVOICE_ID"`
}

// invoiceSearchKey is GetInvoiceRequest/INVOICE_SEARCH_KEY in XSD order.
type invoiceSearchKey struct {
	Limit        int    `xml:"LIMIT,omitempty"`
	ID           string `xml:"ID,omitempty"`
	UUID         string `xml:"UUID,omitempty"`
	DateType     string `xml:"DATE_TYPE,omitempty"`
	StartDate    string `xml:"START_DATE,omitempty"`
	EndDate      string `xml:"END_DATE,omitempty"`
	ReadIncluded *bool  `xml:"READ_INCLUDED,omitempty"`
	Direction    string `xml:"DIRECTION,omitempty"`
}

type getInvoiceRequest struct {
	XMLName    xml.Name         `xml:"wsdl:GetInvoiceRequest"`
	NS         string           `xml:"xmlns:wsdl,attr"`
	Header     requestHeader    `xml:"REQUEST_HEADER"`
	SearchKey  invoiceSearchKey `xml:"INVOICE_SEARCH_KEY"`
	HeaderOnly string           `xml:"HEADER_ONLY,omitempty"`
}

// withTypeSearchKey is GetInvoiceWithTypeRequest/INVOICE_SEARCH_KEY.
type withTypeSearchKey struct {
	UUID         string `xml:"UUID,omitempty"`
	Type         string `xml:"TYPE,omitempty"`
	ReadIncluded *bool  `xml:"READ_INCLUDED,omitempty"`
	Direction    string `xml:"DIRECTION,omitempty"`
}

type getInvoiceWithTypeRequest struct {
	XMLName    xml.Name          `xml:"wsdl:GetInvoiceWithTypeRequest"`
	NS         string            `xml:"xmlns:wsdl,attr"`
	Header     requestHeader     `xml:"REQUEST_HEADER"`
	SearchKey  withTypeSearchKey `xml:"INVOICE_SEARCH_KEY"`
	HeaderOnly string            `xml:"HEADER_ONLY,omitempty"`
}

// amount is c:AmountType.
type amount struct {
	Value    string `xml:",chardata"`
	Currency string `xml:"currencyID,attr"`
}

// invoiceHeader is INVOICE/HEADER (and the status fields that the status
// responses add next to it).
type invoiceHeader struct {
	Sender          string `xml:"SENDER"`
	Receiver        string `xml:"RECEIVER"`
	Supplier        string `xml:"SUPPLIER"`
	Customer        string `xml:"CUSTOMER"`
	IssueDate       string `xml:"ISSUE_DATE"`
	Payable         amount `xml:"PAYABLE_AMOUNT"`
	From            string `xml:"FROM"`
	To              string `xml:"TO"`
	ProfileID       string `xml:"PROFILEID"`
	TypeCode        string `xml:"INVOICE_TYPE_CODE"`
	Status          string `xml:"STATUS"`
	StatusDesc      string `xml:"STATUS_DESCRIPTION"`
	GIBStatusCode   string `xml:"GIB_STATUS_CODE"`
	GIBStatusDesc   string `xml:"GIB_STATUS_DESCRIPTION"`
	Direction       string `xml:"DIRECTION"`
	ResponseCode    string `xml:"RESPONSE_CODE"`
	ResponseDesc    string `xml:"RESPONSE_DESCRIPTION"`
	CDate           string `xml:"CDATE"`
	EnvelopeID      string `xml:"ENVELOPE_IDENTIFIER"`
	StatusCode      string `xml:"STATUS_CODE"`
	StatusDescShort string `xml:"STATUS_DESC"`
}

// merge fills empty fields of h from o.
func (h *invoiceHeader) merge(o invoiceHeader) {
	set := func(dst *string, v string) {
		if strings.TrimSpace(*dst) == "" {
			*dst = v
		}
	}
	set(&h.Status, o.Status)
	set(&h.StatusDesc, o.StatusDesc)
	set(&h.GIBStatusCode, o.GIBStatusCode)
	set(&h.GIBStatusDesc, o.GIBStatusDesc)
	set(&h.Direction, o.Direction)
	set(&h.ResponseCode, o.ResponseCode)
	set(&h.ResponseDesc, o.ResponseDesc)
	set(&h.CDate, o.CDate)
	set(&h.EnvelopeID, o.EnvelopeID)
	set(&h.StatusCode, o.StatusCode)
}

// invoice is the INVOICE element in responses.
type invoice struct {
	ID      string        `xml:"ID,attr"`
	UUID    string        `xml:"UUID,attr"`
	Header  invoiceHeader `xml:"HEADER"`
	Content string        `xml:"CONTENT"`
}

type getInvoiceResponse struct {
	result
	Invoices []invoice `xml:"INVOICE"`
}

type getInvoiceStatusRequest struct {
	XMLName xml.Name      `xml:"wsdl:GetInvoiceStatusRequest"`
	NS      string        `xml:"xmlns:wsdl,attr"`
	Header  requestHeader `xml:"REQUEST_HEADER"`
	Invoice invoiceRef    `xml:"INVOICE"`
}

// invoiceStatus is INVOICE_STATUS. The XSD adds the status fields next to
// HEADER; the vendor's published response samples carry them inside
// HEADER. Both are read.
type invoiceStatus struct {
	ID     string        `xml:"ID,attr"`
	UUID   string        `xml:"UUID,attr"`
	Header invoiceHeader `xml:"HEADER"`
	invoiceHeader
}

func (s *invoiceStatus) fields() invoiceHeader {
	h := s.Header
	h.merge(s.invoiceHeader)
	return h
}

type getInvoiceStatusResponse struct {
	result
	Status *invoiceStatus `xml:"INVOICE_STATUS"`
}

type markInvoiceRequest struct {
	XMLName xml.Name      `xml:"wsdl:MarkInvoiceRequest"`
	NS      string        `xml:"xmlns:wsdl,attr"`
	Header  requestHeader `xml:"REQUEST_HEADER"`
	Mark    mark          `xml:"MARK"`
}

type mark struct {
	Value    string       `xml:"value,attr"`
	Invoices []invoiceRef `xml:"INVOICE"`
}

type markInvoiceResponse struct {
	result
}

type serverSignResponseRequest struct {
	XMLName     xml.Name      `xml:"wsdl:SendInvoiceResponseWithServerSignRequest"`
	NS          string        `xml:"xmlns:wsdl,attr"`
	Header      requestHeader `xml:"REQUEST_HEADER"`
	Status      string        `xml:"STATUS"`
	Invoices    []invoiceRef  `xml:"INVOICE"`
	Description []string      `xml:"DESCRIPTION,omitempty"`
}

type serverSignResponseResponse struct {
	result
}

// EIArchiveWS.

type archiveExtendedRequest struct {
	XMLName xml.Name               `xml:"arc:ArchiveInvoiceExtendedRequest"`
	NS      string                 `xml:"xmlns:arc,attr"`
	Header  requestHeader          `xml:"REQUEST_HEADER"`
	Content archiveExtendedContent `xml:"ArchiveInvoiceExtendedContent"`
}

type archiveExtendedContent struct {
	Properties []archiveInvoiceProperties `xml:"INVOICE_PROPERTIES"`
}

type archiveInvoiceProperties struct {
	EArchiveFlag string            `xml:"EARSIV_FLAG"`
	Properties   archiveProperties `xml:"EARSIV_PROPERTIES"`
	// PDF_PROPERTIES and ARCHIVE_NOTE (not sent) sit here in the XSD.
	Content content `xml:"INVOICE_CONTENT"`
}

// archiveProperties is EARSIV_PROPERTIES in XSD order.
type archiveProperties struct {
	Type      string   `xml:"EARSIV_TYPE"`
	EmailFlag string   `xml:"EARSIV_EMAIL_FLAG,omitempty"`
	Email     []string `xml:"EARSIV_EMAIL,omitempty"`
	Series    string   `xml:"SERI,omitempty"`
	XSLTName  string   `xml:"XSLT_NAME,omitempty"`
}

type archiveExtendedResponse struct {
	result
	InvoiceID string `xml:"INVOICE_ID"`
	WebKey    string `xml:"WEB_KEY"`
}

type archiveReadRequest struct {
	XMLName   xml.Name      `xml:"arc:ArchiveInvoiceReadRequest"`
	NS        string        `xml:"xmlns:arc,attr"`
	Header    requestHeader `xml:"REQUEST_HEADER"`
	InvoiceID string        `xml:"INVOICEID"`
	Direction string        `xml:"PORTAL_DIRECTION"`
	Profile   string        `xml:"PROFILE"`
}

type archiveReadResponse struct {
	Invoices []string `xml:"INVOICE"`
	result
}

type archiveStatusRequest struct {
	XMLName xml.Name      `xml:"arc:GetEArchiveInvoiceStatusRequest"`
	NS      string        `xml:"xmlns:arc,attr"`
	Header  requestHeader `xml:"REQUEST_HEADER"`
	UUID    []string      `xml:"UUID"`
}

type archiveInvoiceHeader struct {
	InvoiceID   string `xml:"INVOICE_ID"`
	Profile     string `xml:"PROFILE"`
	UUID        string `xml:"UUID"`
	InvoiceDate string `xml:"INVOICE_DATE"`
	Status      string `xml:"STATUS"`
	StatusDesc  string `xml:"STATUS_DESC"`
	EmailStatus string `xml:"EMAIL_STATUS"`
	EmailDesc   string `xml:"EMAIL_STATUS_DESC"`
	ReportID    string `xml:"REPORT_ID"`
	WebKey      string `xml:"WEB_KEY"`
}

type archiveStatusResponse struct {
	Invoices []struct {
		Header archiveInvoiceHeader `xml:"HEADER"`
	} `xml:"INVOICE"`
	result
}

type archiveCancelRequest struct {
	XMLName xml.Name               `xml:"arc:CancelEArchiveInvoiceRequest"`
	NS      string                 `xml:"xmlns:arc,attr"`
	Header  requestHeader          `xml:"REQUEST_HEADER"`
	Content []archiveCancelContent `xml:"CancelEArsivInvoiceContent"`
}

// archiveCancelContent is CancelEArsivInvoiceContent in XSD order.
type archiveCancelContent struct {
	UUID string `xml:"FATURA_UUID"`
	Note string `xml:"IPTAL_NOTU,omitempty"`
}

type archiveCancelResponse struct {
	result
}

// EIrsaliyeWS.

type despatchRef struct {
	ID      string   `xml:"ID,attr,omitempty"`
	UUID    string   `xml:"UUID,attr,omitempty"`
	Content *content `xml:"CONTENT,omitempty"`
}

type sendDespatchRequest struct {
	XMLName  xml.Name      `xml:"wsdl:SendDespatchAdviceRequest"`
	NS       string        `xml:"xmlns:wsdl,attr"`
	Header   requestHeader `xml:"REQUEST_HEADER"`
	Sender   *party        `xml:"SENDER,omitempty"`
	Receiver *party        `xml:"RECEIVER,omitempty"`
	Despatch []despatchRef `xml:"DESPATCHADVICE"`
}

type sendDespatchResponse struct {
	DespatchID string `xml:"DESPATCH_ID"`
	result
}

type despatchStatusRequest struct {
	XMLName xml.Name      `xml:"wsdl:GetDespatchAdviceStatusRequest"`
	NS      string        `xml:"xmlns:wsdl,attr"`
	Header  requestHeader `xml:"REQUEST_HEADER"`
	UUID    []string      `xml:"UUID"`
}

// despatchHeader is DESPATCHADVICEHEADER.
type despatchHeader struct {
	ID            string `xml:"ID"`
	UUID          string `xml:"UUID"`
	ProfileID     string `xml:"PROFILEID"`
	Direction     string `xml:"DIRECTION"`
	Status        string `xml:"STATUS"`
	StatusCode    string `xml:"STATUS_CODE"`
	StatusDesc    string `xml:"STATUS_DESCRIPTION"`
	GIBStatusCode string `xml:"GIB_STATUS_CODE"`
	GIBStatusDesc string `xml:"GIB_STATUS_DESCRIPTION"`
	ResponseCode  string `xml:"RESPONSE_CODE"`
	CDate         string `xml:"CDATE"`
	EnvelopeID    string `xml:"ENVELOPE_IDENTIFIER"`
}

type despatchStatusResponse struct {
	Statuses []despatchHeader `xml:"DESPATCHADVICE_STATUS"`
	result
}

// atoi parses an optional integer element, returning 0 when empty or
// malformed.
func atoi(s string) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0
	}
	return n
}
