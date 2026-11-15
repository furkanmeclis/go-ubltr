package sovos

import "encoding/xml"

// e-Fatura: ClientEInvoiceServicesTypes-2.2.xsd, target namespace
// "http:/fitcons.com/eInvoice/" (single slash, as published), qualified
// elements. The namespace is spelled out in each struct tag below.

type sendUBLRequest struct {
	XMLName            xml.Name `xml:"http:/fitcons.com/eInvoice/ sendUBLRequest"`
	VKN                string   `xml:"VKN_TCKN"`
	SenderIdentifier   string   `xml:"SenderIdentifier,omitempty"`
	ReceiverIdentifier string   `xml:"ReceiverIdentifier,omitempty"`
	DocType            string   `xml:"DocType"`
	Parameters         []string `xml:"Parameters,omitempty"`
	DocData            string   `xml:"DocData"`
}

type sendUBLResponse struct {
	Response []struct {
		EnvUUID   string `xml:"EnvUUID"`
		UUID      string `xml:"UUID"`
		ID        string `xml:"ID"`
		CustInvID string `xml:"CustInvID"`
	} `xml:"Response"`
}

type getUBLListRequest struct {
	XMLName    xml.Name `xml:"http:/fitcons.com/eInvoice/ getUBLListRequest"`
	Identifier string   `xml:"Identifier"`
	VKN        string   `xml:"VKN_TCKN"`
	UUID       []string `xml:"UUID,omitempty"`
	DocType    string   `xml:"DocType"`
	Type       string   `xml:"Type"`
	FromDate   string   `xml:"FromDate,omitempty"`
	ToDate     string   `xml:"ToDate,omitempty"`
}

type getUBLListResponse struct {
	UBLList []ublListItem `xml:"UBLList"`
}

type ublListItem struct {
	UUID           string `xml:"UUID"`
	Identifier     string `xml:"Identifier"`
	VKN            string `xml:"VKN_TCKN"`
	EnvType        string `xml:"EnvType"`
	EnvUUID        string `xml:"EnvUUID"`
	ID             string `xml:"ID"`
	CustInvID      string `xml:"CustInvID"`
	InsertDateTime string `xml:"InsertDateTime"`
}

type getUBLRequest struct {
	XMLName    xml.Name `xml:"http:/fitcons.com/eInvoice/ getUBLRequest"`
	Identifier string   `xml:"Identifier"`
	VKN        string   `xml:"VKN_TCKN"`
	UUID       []string `xml:"UUID"`
	DocType    string   `xml:"DocType"`
	Type       string   `xml:"Type"`
	Parameters []string `xml:"Parameters,omitempty"`
}

type docDataResponse struct {
	DocData []string `xml:"DocData"`
}

type getEnvelopeStatusRequest struct {
	XMLName    xml.Name `xml:"http:/fitcons.com/eInvoice/ getEnvelopeStatusRequest"`
	Identifier string   `xml:"Identifier"`
	VKN        string   `xml:"VKN_TCKN"`
	UUID       []string `xml:"UUID"`
}

type getEnvelopeStatusResponse struct {
	Response []struct {
		UUID             string `xml:"UUID"`
		IssueDate        string `xml:"IssueDate"`
		DocumentTypeCode string `xml:"DocumentTypeCode"`
		DocumentType     string `xml:"DocumentType"`
		ResponseCode     string `xml:"ResponseCode"`
		Description      string `xml:"Description"`
	} `xml:"Response"`
}

type getInvoiceViewRequest struct {
	XMLName    xml.Name `xml:"http:/fitcons.com/eInvoice/ getInvoiceViewRequest"`
	UUID       string   `xml:"UUID"`
	CustInvID  string   `xml:"CustInvID"`
	Identifier string   `xml:"Identifier"`
	VKN        string   `xml:"VKN_TCKN"`
	Type       string   `xml:"Type"`
	DocType    string   `xml:"DocType"`
}

// e-Arşiv: EArchiveInvoiceService_v2.wsdl. Operation elements are in the
// invoice / invoicecancellation namespaces, children are unqualified, so
// each request binds a prefix and resets the default namespace.
const (
	nsArcInvoice = "http://fitcons.com/earchive/invoice"
	nsArcCancel  = "http://fitcons.com/earchive/invoicecancellation"
)

type nsAttrs struct {
	Inv string `xml:"xmlns:inv,attr"`
	Def string `xml:"xmlns,attr"`
}

type sendInvoiceRequest struct {
	XMLName xml.Name `xml:"inv:sendInvoiceRequestType"`
	nsAttrs
	SenderID            string               `xml:"senderID"`
	ReceiverID          string               `xml:"receiverID"`
	DocType             string               `xml:"docType"`
	FileName            string               `xml:"fileName"`
	Hash                string               `xml:"hash"`
	BinaryData          string               `xml:"binaryData"`
	CustomizationParams []customizationParam `xml:"customizationParams,omitempty"`
}

type customizationParam struct {
	Name  string `xml:"paramName"`
	Value string `xml:"paramValue"`
}

type sendInvoiceResponse struct {
	Detail  string     `xml:"Detail"`
	Result  string     `xml:"Result>Result"`
	Errors  []preCheck `xml:"preCheckErrorResults>preCheckError"`
	Success []preCheck `xml:"preCheckSuccessResults>preCheckSuccess"`
}

type preCheck struct {
	UUID          string `xml:"UUID"`
	VKN           string `xml:"Vkn"`
	InvoiceNumber string `xml:"InvoiceNumber"`
	ErrorCode     string `xml:"ErrorCode"`
	ErrorDesc     string `xml:"ErrorDesc"`
	SuccessCode   string `xml:"SuccessCode"`
	SuccessDesc   string `xml:"SuccessDesc"`
}

// getStatus / getSignedInvoice / getInvoiceDocument take the same lookup
// fields; outputType only exists on getInvoiceDocument.
type invoiceLookup struct {
	XMLName xml.Name
	nsAttrs
	UUID          string `xml:"UUID"`
	VKN           string `xml:"vkn"`
	InvoiceNumber string `xml:"invoiceNumber,omitempty"`
	OutputType    string `xml:"outputType,omitempty"`
}

type invoiceLookupResponse struct {
	UUID          string `xml:"UUID"`
	InvoiceNumber string `xml:"invoiceNumber"`
	StatusCode    string `xml:"statusCode"`
	Detail        string `xml:"Detail"`
	Hash          string `xml:"Hash"`
	BinaryData    string `xml:"binaryData"`
}

type cancelRequest struct {
	XMLName xml.Name `xml:"inv:invoiceCancellationServiceRequestType"`
	nsAttrs
	List []cancelInfo `xml:"invoiceCancelInfoTypeList"`
}

type cancelInfo struct {
	InvoiceID   string `xml:"invoiceId"`
	VKN         string `xml:"vkn"`
	Branch      string `xml:"branch,omitempty"`
	TotalAmount string `xml:"totalAmount"`
	CancelDate  string `xml:"cancelDate"`
	CustInvID   string `xml:"custInvID,omitempty"`
}

type cancelResponse struct {
	Result       string `xml:"Result>Result"`
	Cancellation []struct {
		Message string `xml:"message"`
		Code    string `xml:"code"`
	} `xml:"invoiceCancellation"`
}
