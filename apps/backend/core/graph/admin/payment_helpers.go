package admin

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/payment"
	"github.com/skeletg/skelet/apps/backend/core/internal/plugin"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
)

// Re-reads after a mutation so the returned row carries the hydrated country allow-list, same as the list query builds it
func (r *mutationResolver) paymentMethodModel(ctx context.Context, channelID uuid.UUID, slug string) (*model.PaymentMethodAdmin, error) {
	providers, err := r.PaymentSvc.ListProvidersForChannel(ctx, channelID)
	if err != nil {
		return nil, err
	}
	for _, p := range providers {
		if p.Slug == slug {
			return toPaymentMethodAdminModel(p), nil
		}
	}
	return nil, fmt.Errorf("payment provider %q not found for channel", slug)
}

func toPaymentMethodAdminModel(p payment.ProviderWithMethod) *model.PaymentMethodAdmin {
	displayName := p.Method.DisplayName
	if displayName == "" {
		displayName = p.DisplayName // method row not synced yet, so fall back to the registry's name
	}
	mode := p.Method.CountryMode
	if mode == "" {
		mode = "all" // the UI always wants a concrete mode
	}
	return &model.PaymentMethodAdmin{
		IsOffline:        p.IsOffline,
		IsAvailable:      p.IsAvailable,
		ProviderSlug:     p.Slug,
		DisplayName:      displayName,
		IsEnabled:        p.Method.IsEnabled,
		SortOrder:        p.Method.SortOrder,
		CountryMode:      mode,
		AllowedCountries: append([]string{}, p.Method.AllowedCountries...),
	}
}

// Metadata is registry-owned. Config values, especially secrets, remain behind the config API's own permissions.
func paymentProviderModels(ctx context.Context, channelID uuid.UUID) []*model.PaymentProviderAdmin {
	out := make([]*model.PaymentProviderAdmin, 0)
	for _, reg := range payment.AllProviders() {
		m := &model.PaymentProviderAdmin{Slug: reg.Slug, DisplayName: reg.DisplayName, IsOffline: reg.IsOffline,
			PluginSlug: reg.PluginSlug, IsAvailable: payment.ProviderAvailable(ctx, reg, channelID), ConfigurationPaths: []string{}}
		for _, manifest := range plugin.Manifests() {
			if manifest.Slug == reg.PluginSlug {
				m.PluginName, m.PluginVersion = manifest.Name, manifest.Version
				break
			}
		}
		for _, def := range config.AllDefinitions() {
			if strings.HasPrefix(def.Path, "payment/"+reg.Slug+"/") {
				m.ConfigurationPaths = append(m.ConfigurationPaths, def.Path)
			}
		}
		if reg.PluginSlug != "" {
			path := "plugin/" + reg.PluginSlug + "/enabled"
			if _, ok := config.Find(path); ok {
				m.ConfigurationPaths = append(m.ConfigurationPaths, path)
			}
		}
		if !reg.IsOffline {
			path := "/api/v1/payments/webhook/" + url.PathEscape(reg.Slug)
			if channelID != uuid.Nil {
				path += "?channel=" + channelID.String()
			}
			m.WebhookPath = &path
		}
		out = append(out, m)
	}
	return out
}
