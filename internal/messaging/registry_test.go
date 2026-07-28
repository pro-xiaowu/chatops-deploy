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
	checkErr   error
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
	if p.checkErr != nil {
		return p.checkErr
	}
	if !p.configured {
		return errors.New("not configured")
	}
	return nil
}

func TestRegistryDoesNotSelectUnhealthyProvider(t *testing.T) {
	store := &storeStub{active: domain.MessageProviderWeb}
	providerErr := errors.New("invalid credentials")
	registry := messaging.NewRegistry(store, []messaging.Provider{fakeProvider{name: domain.MessageProviderWeCom, configured: true, checkErr: providerErr}})

	err := registry.Select(context.Background(), domain.MessageProviderWeCom)

	require.ErrorContains(t, err, "invalid credentials")
	require.Equal(t, domain.MessageProviderWeb, store.active)
	capabilities := registry.Capabilities(context.Background())
	require.Len(t, capabilities, 1)
	require.False(t, capabilities[0].Healthy)
	require.Equal(t, "invalid credentials", capabilities[0].Reason)
}

func TestRegistryCachesSuccessfulHealthCheck(t *testing.T) {
	store := &storeStub{active: domain.MessageProviderWeb}
	registry := messaging.NewRegistry(store, []messaging.Provider{fakeProvider{name: domain.MessageProviderDingTalk, configured: true}})

	before := registry.Capabilities(context.Background())
	require.False(t, before[0].Healthy)
	require.NoError(t, registry.Select(context.Background(), domain.MessageProviderDingTalk))
	after := registry.Capabilities(context.Background())
	require.True(t, after[0].Healthy)
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
