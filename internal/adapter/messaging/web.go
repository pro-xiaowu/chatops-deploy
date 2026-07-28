package messaging

import (
	"context"
	"errors"

	"chatops-deploy/internal/domain"
	"chatops-deploy/internal/messaging"
)

type Provider struct{}

func New() *Provider                          { return &Provider{} }
func (Provider) Name() domain.MessageProvider { return domain.MessageProviderWeb }
func (Provider) Capabilities(context.Context) messaging.Capabilities {
	return messaging.Capabilities{Provider: domain.MessageProviderWeb, Configured: true, Healthy: true}
}
func (Provider) Decode(context.Context, messaging.Request) (messaging.Incoming, error) {
	return messaging.Incoming{}, messaging.ErrUnsupportedDecode
}
func (Provider) Send(context.Context, messaging.Notification) error {
	return errors.New("web provider does not send messages")
}
func (Provider) Check(context.Context) error { return nil }
