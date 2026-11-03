package wiring

import "github.com/skeletg/skelet/apps/backend/core/internal/agenttools"

// One tool menu for the admin chat and the MCP endpoint, so the two can't drift on what a caller may read
func newAgentTools(infra Infra, s Svcs) *agenttools.Service {
	return agenttools.New(agentToolDeps(infra, s))
}

func agentToolDeps(infra Infra, s Svcs) agenttools.Deps {
	return agenttools.Deps{
		Admin:          s.Admin,
		AI:             s.AI,
		Catalog:        s.Catalog,
		Channel:        s.Channel,
		CMS:            s.CMS,
		Currency:       s.Currency,
		Customer:       s.Customer,
		Giftcard:       s.Giftcard,
		Inventory:      s.Inventory,
		Locale:         s.Locale,
		Menu:           s.Menu,
		Order:          s.Order,
		Promotion:      s.Promotion,
		Review:         s.Review,
		Sales:          s.Sales,
		Shipping:       s.Shipping,
		Subscription:   s.Subscription,
		Tax:            s.Tax,
		URLRewrite:     s.URLRewrite,
		Signals:        s.SignalsClient,
		SignalsEnabled: bool(s.SignalsEnabled),
		DefaultLocale:  NewCatalogDefaultLocaleProvider(s.Channel),
		Attributes:     infra.Queries,
	}
}
