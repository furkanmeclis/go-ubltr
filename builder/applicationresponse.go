package builder

import (
	"strings"

	"github.com/furkanmeclis/go-ubltr/schema/ubltr121/applicationresponse"
	"github.com/furkanmeclis/go-ubltr/schema/ubltr121/cac"
	"github.com/furkanmeclis/go-ubltr/schema/ubltr121/cbc"
)

// ApplicationResponseBuilder constructs a generated ApplicationResponse.
type ApplicationResponseBuilder struct {
	err error

	profile, id, uuid string
	issueDate         string
	sender, receiver  cac.PartyType
	hasSender         bool
	hasReceiver       bool
	responses         []cac.DocumentResponseType
}

// NewApplicationResponseBuilder returns an empty application-response builder.
func NewApplicationResponseBuilder() *ApplicationResponseBuilder {
	return &ApplicationResponseBuilder{
		responses: []cac.DocumentResponseType{},
	}
}

func (b *ApplicationResponseBuilder) fail(field, msg string) {
	if b == nil || b.err != nil {
		return
	}
	b.err = builderErr(field, msg)
}

func (b *ApplicationResponseBuilder) failErr(field string, err error) {
	if b == nil || b.err != nil || err == nil {
		return
	}
	b.err = builderWrap(field, err)
}

// Profile sets ProfileID.
func (b *ApplicationResponseBuilder) Profile(id string) *ApplicationResponseBuilder {
	b.profile = strings.TrimSpace(id)
	return b
}

// ID sets the document ID.
func (b *ApplicationResponseBuilder) ID(id string) *ApplicationResponseBuilder {
	b.id = strings.TrimSpace(id)
	return b
}

// UUID sets cbc:UUID. Generated on Build when empty.
func (b *ApplicationResponseBuilder) UUID(id string) *ApplicationResponseBuilder {
	b.uuid = strings.TrimSpace(id)
	return b
}

// IssueDate sets IssueDate (YYYY-MM-DD).
func (b *ApplicationResponseBuilder) IssueDate(date string) *ApplicationResponseBuilder {
	b.issueDate = strings.TrimSpace(date)
	return b
}

// Sender sets SenderParty.
func (b *ApplicationResponseBuilder) Sender(p cac.PartyType) *ApplicationResponseBuilder {
	b.sender = p
	b.hasSender = true
	return b
}

// SenderBuilder builds the party and sets the sender.
func (b *ApplicationResponseBuilder) SenderBuilder(pb *PartyBuilder) *ApplicationResponseBuilder {
	p, err := pb.Build()
	if err != nil {
		b.failErr("sender", err)
		return b
	}
	return b.Sender(p)
}

// Receiver sets ReceiverParty.
func (b *ApplicationResponseBuilder) Receiver(p cac.PartyType) *ApplicationResponseBuilder {
	b.receiver = p
	b.hasReceiver = true
	return b
}

// ReceiverBuilder builds the party and sets the receiver.
func (b *ApplicationResponseBuilder) ReceiverBuilder(pb *PartyBuilder) *ApplicationResponseBuilder {
	p, err := pb.Build()
	if err != nil {
		b.failErr("receiver", err)
		return b
	}
	return b.Receiver(p)
}

// DocumentResponse appends a response for documentID issued on issueDate.
// code is typically [codelist.ResponseKABUL] or [codelist.ResponseRED].
func (b *ApplicationResponseBuilder) DocumentResponse(documentID, issueDate, code string) *ApplicationResponseBuilder {
	b.responses = append(b.responses, cac.DocumentResponseType{
		Response: cac.ResponseType{
			ReferenceID:  cbc.ReferenceIDType{Value: documentID},
			ResponseCode: ptrOf(cbc.ResponseCodeType{Value: code}),
		},
		DocumentReference: cac.DocumentReferenceType{
			ID:        cbcID(documentID),
			IssueDate: cbc.IssueDateType{Value: issueDate},
		},
	})
	return b
}

// Build returns a generated *applicationresponse.ApplicationResponse.
func (b *ApplicationResponseBuilder) Build() (*applicationresponse.ApplicationResponse, error) {
	if b == nil {
		return nil, builderErr("applicationresponse", "nil builder")
	}
	if b.err != nil {
		return nil, b.err
	}
	if b.profile == "" {
		b.fail("profile", "required")
	}
	if b.id == "" {
		b.fail("id", "required")
	}
	if b.issueDate == "" {
		b.fail("issueDate", "required")
	}
	if !b.hasSender {
		b.fail("sender", "required")
	}
	if !b.hasReceiver {
		b.fail("receiver", "required")
	}
	if len(b.responses) == 0 {
		b.fail("documentResponse", "at least one document response is required")
	}
	if b.err != nil {
		return nil, b.err
	}

	uuid := b.uuid
	if uuid == "" {
		id, err := newUUID()
		if err != nil {
			return nil, builderWrap("uuid", err)
		}
		uuid = id
	}

	doc := &applicationresponse.ApplicationResponse{}
	doc.UBLExtensions = ptrExtensions()
	doc.UBLVersionID = cbc.UBLVersionIDType{Value: ublVersionTR}
	doc.CustomizationID = cbc.CustomizationIDType{Value: customizationTR12}
	doc.ProfileID = cbc.ProfileIDType{Value: b.profile}
	doc.ID = cbcID(b.id)
	doc.UUID = cbc.UUIDType{Value: uuid}
	doc.IssueDate = cbc.IssueDateType{Value: b.issueDate}
	doc.SenderParty = b.sender
	doc.ReceiverParty = b.receiver
	doc.DocumentResponse = b.responses
	doc.Signature = []cac.SignatureType{signatureOf(b.sender)}
	return doc, nil
}
