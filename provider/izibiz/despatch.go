package izibiz

import (
	"context"
	"strings"

	"github.com/furkanmeclis/go-ubltr/provider"
)

func (p *Provider) sendDespatch(ctx context.Context, doc *provider.Document) (*provider.SendResult, error) {
	c, err := zipContent(doc)
	if err != nil {
		return nil, err
	}
	var sender, receiver *party
	if doc.SenderAlias != "" {
		sender = &party{Alias: doc.SenderAlias}
	}
	if doc.ReceiverVKN != "" || doc.ReceiverAlias != "" {
		receiver = &party{VKN: doc.ReceiverVKN, Alias: doc.ReceiverAlias}
	}
	resp, err := invoke[sendDespatchResponse](ctx, p, p.despatch, "send", func(h requestHeader) any {
		return sendDespatchRequest{NS: nsWSDL, Header: h, Sender: sender, Receiver: receiver, Despatch: []despatchRef{{Content: c}}}
	}, true)
	if err != nil {
		return nil, err
	}
	id := strings.TrimSpace(resp.DespatchID)
	if id == "" {
		id = doc.ID
	}
	return &provider.SendResult{UUID: doc.UUID, ID: id, Status: provider.StatusQueued, ProviderCode: returnCode(resp.Return)}, nil
}

func (p *Provider) despatchStatus(ctx context.Context, uuid string) (*provider.StatusResult, error) {
	resp, err := invoke[despatchStatusResponse](ctx, p, p.despatch, "status", func(h requestHeader) any {
		return despatchStatusRequest{NS: nsWSDL, Header: h, UUID: []string{uuid}}
	}, false)
	if err != nil {
		return nil, err
	}
	for _, s := range resp.Statuses {
		if s.UUID != "" && !strings.EqualFold(s.UUID, uuid) {
			continue
		}
		gib := atoi(s.GIBStatusCode)
		code := firstNonEmpty(strings.TrimSpace(s.StatusCode), strings.TrimSpace(s.Status))
		return &provider.StatusResult{
			UUID:         firstNonEmpty(s.UUID, uuid),
			ID:           strings.TrimSpace(s.ID),
			Status:       documentStatus(s.StatusCode, s.Status, gib),
			GIBCode:      gib,
			ProviderCode: code,
			Message:      strings.TrimSpace(s.StatusDesc),
		}, nil
	}
	return nil, notFound("status", uuid)
}
