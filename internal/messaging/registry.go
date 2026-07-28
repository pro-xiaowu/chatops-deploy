package messaging

import (
	"context"
	"fmt"

	"chatops-deploy/internal/domain"
)

type ProviderSettingsStore interface {
	GetActiveMessageProvider(context.Context) (domain.MessageProvider, error)
	SetActiveMessageProvider(context.Context, domain.MessageProvider) error
	HasUnfinishedMessageOperations(context.Context, domain.MessageProvider) (bool, error)
}

type Registry struct {
	store     ProviderSettingsStore
	providers map[domain.MessageProvider]Provider
}

func NewRegistry(store ProviderSettingsStore, providers []Provider) *Registry {
	byName := make(map[domain.MessageProvider]Provider, len(providers))
	for _, provider := range providers {
		byName[provider.Name()] = provider
	}
	return &Registry{store: store, providers: byName}
}

func (r *Registry) Active(ctx context.Context) (Provider, error) {
	name, err := r.store.GetActiveMessageProvider(ctx)
	if err != nil {
		return nil, err
	}
	provider, ok := r.providers[name]
	if !ok {
		return nil, fmt.Errorf("message provider %s is not available", name)
	}
	return provider, nil
}

func (r *Registry) Provider(name domain.MessageProvider) (Provider, bool) {
	provider, ok := r.providers[name]
	return provider, ok
}

func (r *Registry) Select(ctx context.Context, name domain.MessageProvider) error {
	provider, ok := r.providers[name]
	if !ok {
		return fmt.Errorf("message provider %s is not available", name)
	}
	capabilities := provider.Capabilities(ctx)
	if !capabilities.Configured {
		return fmt.Errorf("message provider %s is not configured", name)
	}
	return r.store.SetActiveMessageProvider(ctx, name)
}

func (r *Registry) Capabilities(ctx context.Context) []Capabilities {
	out := make([]Capabilities, 0, len(r.providers))
	for _, name := range []domain.MessageProvider{domain.MessageProviderWeb, domain.MessageProviderFeishu, domain.MessageProviderWeCom, domain.MessageProviderDingTalk} {
		if provider, ok := r.providers[name]; ok {
			out = append(out, provider.Capabilities(ctx))
		}
	}
	return out
}
