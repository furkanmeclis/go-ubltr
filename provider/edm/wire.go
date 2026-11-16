package edm

import (
	"encoding/xml"
	"strconv"
	"strings"
)

// Wire types for the EDM EFaturaEDM.svc service. Only the elements used by
// this module are declared. Names, order and qualification follow the
// published WSDL (copies in testdata/wsdl): request and response roots
// are in http://tempuri.org/, their children are form="unqualified".

const nsTempuri = "http://tempuri.org/"

// SOAPAction values from the WSDL binding.
const (
	actionLogin        = "LoginRequest"
	actionLogout       = "LogoutRequest"
	actionSendInvoice  = "SendInvoiceRequest"
	actionGetInvoice   = "GetInvoiceRequest"
	actionStatus       = "GetInvoiceStatusRequest"
	actionMarkInvoice  = "MarkInvoiceRequest"
	actionCheckUser    = "CheckUserRequest"
	contentTypeXML     = "XML"
	directionIncoming  = "IN"
	directionOutgoing  = "OUT"
	headerOnlyYes      = "Y"
	headerOnlyNo       = "N"
	compressedNo       = "N"
	markRead           = "READ"
	actionDateTimeForm = "2006-01-02T15:04:05"
)

// requestHeader is REQUEST_HEADERType in XSD order.
type requestHeader struct {
	SessionID       string `xml:"SESSION_ID,omitempty"`
	ClientTxnID     string `xml:"CLIENT_TXN_ID,omitempty"`
	ActionDate      string `xml:"ACTION_DATE,omitempty"`
	Reason          string `xml:"REASON,omitempty"`
	ApplicationName string `xml:"APPLICATION_NAME,omitempty"`
	Hostname        string `xml:"HOSTNAME,omitempty"`
	ChannelName     string `xml:"CHANNEL_NAME,omitempty"`
	Compressed      string `xml:"COMPRESSED,omitempty"`
}

type requestReturn struct {
	IntlTxnID  string `xml:"INTL_TXN_ID"`
	ReturnCode string `xml:"RETURN_CODE"`
}

// errorType is REQUEST_ERRORType, the RequestFault detail.
type errorType struct {
	Code  string `xml:"ERROR_CODE"`
	Short string `xml:"ERROR_SHORT_DES"`
	Long  string `xml:"ERROR_LONG_DES"`
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

type party struct {
	VKN   string `xml:"vkn,attr,omitempty"`
	Alias string `xml:"alias,attr,omitempty"`
}

type content struct {
	Value string `xml:",chardata"`
}

// invoiceRef is the INVOICE element in requests. The WSDL declares the
// TRXID attribute as required, so it is always written (0 for new
// documents, as a generated WCF client would send).
type invoiceRef struct {
	TRXID   int64    `xml:"TRXID,attr"`
	UUID    string   `xml:"UUID,attr,omitempty"`
	ID      string   `xml:"ID,attr,omitempty"`
	Content *content `xml:"CONTENT,omitempty"`
}

type loginRequest struct {
	XMLName  xml.Name      `xml:"tem:LoginRequest"`
	NS       string        `xml:"xmlns:tem,attr"`
	Header   requestHeader `xml:"REQUEST_HEADER"`
	UserName string        `xml:"USER_NAME"`
	Password string        `xml:"PASSWORD"`
}

type loginResponse struct {
	Return    *requestReturn `xml:"REQUEST_RETURN"`
	SessionID string         `xml:"SESSION_ID"`
}

type logoutRequest struct {
	XMLName xml.Name      `xml:"tem:LogoutRequest"`
	NS      string        `xml:"xmlns:tem,attr"`
	Header  requestHeader `xml:"REQUEST_HEADER"`
}

type sendInvoiceRequest struct {
	XMLName  xml.Name      `xml:"tem:SendInvoiceRequest"`
	NS       string        `xml:"xmlns:tem,attr"`
	Header   requestHeader `xml:"REQUEST_HEADER"`
	Sender   *party        `xml:"SENDER,omitempty"`
	Receiver *party        `xml:"RECEIVER,omitempty"`
	Invoices []invoiceRef  `xml:"INVOICE"`
}

type sendInvoiceResponse struct {
	Return   *requestReturn `xml:"REQUEST_RETURN"`
	Invoices []invoice      `xml:"INVOICE"`
}

// searchKey is GetInvoiceRequest/INVOICE_SEARCH_KEY in XSD order.
type searchKey struct {
	Limit        int    `xml:"LIMIT,omitempty"`
	UUID         string `xml:"UUID,omitempty"`
	ReadIncluded *bool  `xml:"READ_INCLUDED,omitempty"`
	Direction    string `xml:"DIRECTION,omitempty"`
	CrStartDate  string `xml:"CR_START_DATE,omitempty"`
	CrEndDate    string `xml:"CR_END_DATE,omitempty"`
	Offset       int    `xml:"OFFSET,omitempty"`
}

type getInvoiceRequest struct {
	XMLName     xml.Name      `xml:"tem:GetInvoiceRequest"`
	NS          string        `xml:"xmlns:tem,attr"`
	Header      requestHeader `xml:"REQUEST_HEADER"`
	SearchKey   searchKey     `xml:"INVOICE_SEARCH_KEY"`
	HeaderOnly  string        `xml:"HEADER_ONLY,omitempty"`
	ContentType string        `xml:"INVOICE_CONTENT_TYPE"`
}

type amount struct {
	Value    string `xml:",chardata"`
	Currency string `xml:"currencyID,attr"`
}

// invoiceHeader is INVOICE/HEADER; the status fields also appear directly
// under INVOICE_STATUS in GetInvoiceStatusResponse.
type invoiceHeader struct {
	Sender        string `xml:"SENDER"`
	Receiver      string `xml:"RECEIVER"`
	Supplier      string `xml:"SUPPLIER"`
	Customer      string `xml:"CUSTOMER"`
	IssueDate     string `xml:"ISSUE_DATE"`
	Payable       amount `xml:"PAYABLE_AMOUNT"`
	ProfileID     string `xml:"PROFILEID"`
	Status        string `xml:"STATUS"`
	StatusDesc    string `xml:"STATUS_DESCRIPTION"`
	GIBStatusCode string `xml:"GIB_STATUS_CODE"`
	GIBStatusDesc string `xml:"GIB_STATUS_DESCRIPTION"`
	ResponseCode  string `xml:"RESPONSE_CODE"`
	CDate         string `xml:"CDATE"`
	EnvelopeID    string `xml:"ENVELOPE_IDENTIFIER"`
	Direction     string `xml:"DIRECTION"`
}

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
	set(&h.ResponseCode, o.ResponseCode)
	set(&h.CDate, o.CDate)
	set(&h.EnvelopeID, o.EnvelopeID)
	set(&h.Direction, o.Direction)
}

type invoice struct {
	UUID    string        `xml:"UUID,attr"`
	ID      string        `xml:"ID,attr"`
	Header  invoiceHeader `xml:"HEADER"`
	Content string        `xml:"CONTENT"`
}

type getInvoiceResponse struct {
	Invoices []invoice `xml:"INVOICE"`
}

type statusRequest struct {
	XMLName xml.Name      `xml:"tem:GetInvoiceStatusRequest"`
	NS      string        `xml:"xmlns:tem,attr"`
	Header  requestHeader `xml:"REQUEST_HEADER"`
	Invoice invoiceRef    `xml:"INVOICE"`
}

type invoiceStatus struct {
	UUID   string        `xml:"UUID,attr"`
	ID     string        `xml:"ID,attr"`
	Header invoiceHeader `xml:"HEADER"`
	invoiceHeader
}

func (s *invoiceStatus) fields() invoiceHeader {
	h := s.invoiceHeader
	h.merge(s.Header)
	return h
}

type statusResponse struct {
	Return *requestReturn `xml:"REQUEST_RETURN"`
	Status *invoiceStatus `xml:"INVOICE_STATUS"`
}

type markRequest struct {
	XMLName xml.Name      `xml:"tem:MarkInvoiceRequest"`
	NS      string        `xml:"xmlns:tem,attr"`
	Header  requestHeader `xml:"REQUEST_HEADER"`
	Mark    mark          `xml:"MARK"`
}

type mark struct {
	Value    string       `xml:"value,attr"`
	Invoices []invoiceRef `xml:"INVOICE"`
}

type markResponse struct {
	Return *requestReturn `xml:"REQUEST_RETURN"`
}

type checkUserRequest struct {
	XMLName xml.Name      `xml:"tem:CheckUserRequest"`
	NS      string        `xml:"xmlns:tem,attr"`
	Header  requestHeader `xml:"REQUEST_HEADER"`
	User    gibUser       `xml:"USER"`
}

type gibUser struct {
	Identifier   string `xml:"IDENTIFIER,omitempty"`
	Alias        string `xml:"ALIAS,omitempty"`
	Title        string `xml:"TITLE,omitempty"`
	Type         string `xml:"TYPE,omitempty"`
	RegisterTime string `xml:"REGISTER_TIME,omitempty"`
	Unit         string `xml:"UNIT,omitempty"`
	AliasCreated string `xml:"ALIAS_CREATION_TIME,omitempty"`
	AliasRemoved string `xml:"ALIAS_REMOVAL_TIME,omitempty"`
	DocumentType string `xml:"DOCUMENTTYPE,omitempty"`
}

type checkUserResponse struct {
	Users []gibUser `xml:"USER"`
}

func atoi(s string) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0
	}
	return n
}
