package agenttools

import (
	"context"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

// Catalog/Orders gate the page-aware entity tools and are deliberately outside Any() - the chat's entry gate stays Signals-only, these just unlock extra tools for an admin who already got in
type Perms struct {
	Sales, Lens, Synapse, Pulse, Prism         bool
	Customers                                  bool // customers.view on its own - the commerce record, which outlives the suite
	Catalog, Orders                            bool
	Inventory, Promotions, Reviews             bool
	Campaigns, Flows, Subscriptions, Giftcards bool
	Channels, URLRewrite                       bool
	Currencies, Locales, Settings, CMS         bool
}

// Customers rides along so an admin who reached the chat through customers.view keeps it with the suite off, when the record tools are all that is left
func (p Perms) Any() bool {
	return p.Sales || p.Lens || p.Synapse || p.Pulse || p.Prism || p.Customers
}

func (s *Service) Perms(ctx context.Context, cid uuid.UUID) Perms { return s.deps.perms(ctx, cid) }

func (deps *Deps) perms(ctx context.Context, cid uuid.UUID) Perms {
	// Fails closed on its own: LoadPermittedChannels reports Bypass=true with no caller in ctx, which would hand an anonymous caller every section
	if _, ok := auth.ActorFromContext(ctx); !ok {
		return Perms{}
	}
	allows := func(perm string) bool {
		pc, err := auth.LoadPermittedChannels(ctx, deps.Admin, perm)
		return err == nil && (pc.Bypass() || pc.Allows(cid))
	}
	// With the suite off or unaddressed there's no data, and Authed only mints a Signals token for a signed-in admin - integration tokens never get a Signals tool
	_, isAdmin := auth.AdminFromContext(ctx)
	signals := func(perm string) bool { return isAdmin && deps.SignalsEnabled && deps.Signals != nil && allows(perm) }
	return Perms{
		Sales:         allows("reports.view"),
		Lens:          signals("lens.view"),
		Synapse:       signals("customers.view"),
		Pulse:         signals("emails.view"),
		Prism:         signals("experiments.view"),
		Customers:     allows("customers.view"),
		Catalog:       allows("catalog.view"),
		Orders:        allows("orders.view"),
		Inventory:     allows("inventory.view"),
		Promotions:    allows("promotions.view"),
		Reviews:       allows("reviews.view"),
		Campaigns:     signals("campaigns.view"), // Pulse is the suite's, so its tools follow the switch too
		Flows:         signals("flows.view"),
		Subscriptions: allows("subscriptions.view"),
		Giftcards:     allows("giftcards.view"),
		Channels:      allows("channels.view"),
		URLRewrite:    allows("urlrewrite.view"),
		Currencies:    allows("currencies.view"),
		Locales:       allows("locales.view"),
		Settings:      allows("settings.view"),
		CMS:           allows("cms.view"),
	}
}
