package messaging_test

import (
	"context"
	"errors"
	"testing"

	"chatops-deploy/internal/domain"
	"chatops-deploy/internal/messaging"
	"github.com/stretchr/testify/require"
)

type fakeProvider struct {
	name       domain.MessageProvider
	configured bool
}

func (p fakeProvider) Name() domain.MessageProvider { return p.name }
func (p fakeProvider) Capabilities(context.Context) messaging.Capabilities {
	return messaging.Capabilities{Provider: p.name, Configured: p.configured, Healthy: p.configured}
}
func (p fakeProvider) Decode(context.Context, messaging.Request) (messaging.Incoming, error) {
	return messaging.Incoming{}, errors.New("not implemented")
}
func (p fakeProvider) Send(context.Context, messaging.Notification) error { return nil }
func (p fakeProvider) Check(context.Context) error {
	if !p.configured {
		return errors.New("not configured")
	}
	return nil
}

type storeStub struct{ active domain.MessageProvider }

func (s *storeStub) GetActiveMessageProvider(context.Context) (domain.MessageProvider, error) {
	return s.active, nil
}
func (s *storeStub) SetActiveMessageProvider(_ context.Context, provider domain.MessageProvider) error {
	s.active = provider
	return nil
}
func (*storeStub) HasUnfinishedMessageOperations(context.Context, domain.MessageProvider) (bool, error) {
	return false, nil
}

func TestRegistryRejectsProviderWithoutConfiguration(t *testing.T) {
	store := &storeStub{active: domain.MessageProviderWeb}
	registry := messaging.NewRegistry(store, []messaging.Provider{fakeProvider{name: domain.MessageProviderWeb, configured: true}, fakeProvider{name: domain.MessageProviderFeishu, configured: false}})

	require.EqualError(t, registry.Select(context.Background(), domain.MessageProviderFeishu), "message provider feishu is not configured")
}

func TestRegistryCapabilitiesAreStableAndOrdered(t *testing.T) {
	store := &storeStub{active: domain.MessageProviderWeb}
	registry := messaging.NewRegistry(store, []messaging.Provider{fakeProvider{name: domain.MessageProviderFeishu, configured: false}, fakeProvider{name: domain.MessageProviderWeb, configured: true}})

	capabilities := registry.Capabilities(context.Background())
	require.Len(t, capabilities, 2)
	require.Equal(t, domain.MessageProviderWeb, capabilities[0].Provider)
	require.Equal(t, domain.MessageProviderFeishu, capabilities[1].Provider)
}
