package admin

// AI-chat tool definitions and handlers for store config / content (currencies, locales, menus, CMS pages, shipping,
// tax). The shared registry, dispatch and schema helpers live in aichat_tools.go

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/internal/ai"
	"github.com/skeletg/skelet/apps/backend/internal/cms"
)

// storeConfigSignalTools returns the store config / content tools.
func storeConfigSignalTools() []signalTool {
	return []signalTool{
		// ---- Store config / content ----
		{
			def: ai.Tool{
				Name:        "list_currencies",
				Description: "Configured currencies (code, name, symbol, active).",
				Schema:      objSchema(map[string]any{}),
			},
			perm: func(p digestPerms) bool { return p.currencies },
			run: func(ctx context.Context, r *Resolver, _ uuid.UUID, _ time.Time, _ int, _ toolArgs) string {
				return toolListCurrencies(ctx, r)
			},
		},
		{
			def: ai.Tool{
				Name:        "list_locales",
				Description: "Configured locales/languages (code, name, native name, active).",
				Schema:      objSchema(map[string]any{}),
			},
			perm: func(p digestPerms) bool { return p.locales },
			run: func(ctx context.Context, r *Resolver, _ uuid.UUID, _ time.Time, _ int, _ toolArgs) string {
				return toolListLocales(ctx, r)
			},
		},
		{
			def: ai.Tool{
				Name:        "list_menus",
				Description: "Navigation menus defined in the store (name, slug).",
				Schema:      objSchema(map[string]any{}),
			},
			perm: func(p digestPerms) bool { return p.cms },
			run: func(ctx context.Context, r *Resolver, _ uuid.UUID, _ time.Time, _ int, _ toolArgs) string {
				return toolListMenus(ctx, r)
			},
		},
		{
			def: ai.Tool{
				Name:        "list_cms_pages",
				Description: "CMS pages (title, slug, published or draft).",
				Schema:      objSchema(map[string]any{"limit": limitProp("Max pages to return (1-50). Default 25.", 50)}),
			},
			perm: func(p digestPerms) bool { return p.cms },
			run: func(ctx context.Context, r *Resolver, _ uuid.UUID, _ time.Time, _ int, a toolArgs) string {
				return toolListCMSPages(ctx, r, a)
			},
		},
		{
			def: ai.Tool{
				Name:        "list_shipping_methods",
				Description: "Shipping carriers configured for this channel (name, slug, tracking/label support).",
				Schema:      objSchema(map[string]any{}),
			},
			perm: func(p digestPerms) bool { return p.settings },
			run: func(ctx context.Context, r *Resolver, cid uuid.UUID, _ time.Time, _ int, _ toolArgs) string {
				return toolListShipping(ctx, r, cid)
			},
		},
		{
			def: ai.Tool{
				Name:        "list_tax_rules",
				Description: "Tax rules for this channel (name, active, priority).",
				Schema:      objSchema(map[string]any{}),
			},
			perm: func(p digestPerms) bool { return p.settings },
			run: func(ctx context.Context, r *Resolver, cid uuid.UUID, _ time.Time, _ int, _ toolArgs) string {
				return toolListTaxRules(ctx, r, cid)
			},
		},
	}
}

// ---- store config / content formatters ----

func toolListCurrencies(ctx context.Context, r *Resolver) string {
	cur, err := r.CurrencySvc.List(ctx, false)
	if err != nil {
		return "Could not load currencies."
	}
	if len(cur) == 0 {
		return "No currencies configured."
	}
	var b strings.Builder
	b.WriteString("Currencies (code - name - symbol - state):\n")
	for _, c := range cur {
		state := "active"
		if !c.IsActive {
			state = "inactive"
		}
		fmt.Fprintf(&b, "- %s - %s - %s - %s\n", c.Code, c.Name, c.Symbol, state)
	}
	return b.String()
}

func toolListLocales(ctx context.Context, r *Resolver) string {
	locs, err := r.LocaleSvc.List(ctx, false)
	if err != nil {
		return "Could not load locales."
	}
	if len(locs) == 0 {
		return "No locales configured."
	}
	var b strings.Builder
	b.WriteString("Locales (code - name - state):\n")
	for _, l := range locs {
		state := "active"
		if !l.IsActive {
			state = "inactive"
		}
		fmt.Fprintf(&b, "- %s - %s (%s) - %s\n", l.Code, l.Name, l.NativeName, state)
	}
	return b.String()
}

func toolListMenus(ctx context.Context, r *Resolver) string {
	menus, err := r.MenuSvc.ListMenus(ctx)
	if err != nil {
		return "Could not load menus."
	}
	if len(menus) == 0 {
		return "No menus defined."
	}
	var b strings.Builder
	b.WriteString("Menus (name - slug):\n")
	for _, m := range menus {
		fmt.Fprintf(&b, "- %s - %s\n", m.Name, m.Slug)
	}
	return b.String()
}

func toolListCMSPages(ctx context.Context, r *Resolver, a toolArgs) string {
	limit := clampLimit(a.Limit, 25, 50)
	pages, err := r.CMSSvc.ListPages(ctx, cms.PageListFilter{}, r.activeLocale(ctx))
	if err != nil {
		return "Could not load CMS pages."
	}
	if len(pages) == 0 {
		return "No CMS pages."
	}
	var b strings.Builder
	b.WriteString("CMS pages (title - slug - state):\n")
	for i, p := range pages {
		if i >= limit {
			break
		}
		title := p.Slug
		if p.Content != nil && strings.TrimSpace(p.Content.Title) != "" {
			title = p.Content.Title
		}
		state := "draft"
		if p.IsPublished {
			state = "published"
		}
		fmt.Fprintf(&b, "- %s - %s - %s\n", title, p.Slug, state)
	}
	return b.String()
}

func toolListShipping(ctx context.Context, r *Resolver, cid uuid.UUID) string {
	carriers, err := r.ShippingSvc.ListCarriersForChannel(ctx, cid)
	if err != nil {
		return "Could not load shipping methods."
	}
	if len(carriers) == 0 {
		return "No shipping carriers configured for this channel."
	}
	var b strings.Builder
	b.WriteString("Shipping carriers (name - slug - capabilities):\n")
	for _, c := range carriers {
		caps := make([]string, 0, 2)
		if c.SupportsTrack {
			caps = append(caps, "tracking")
		}
		if c.SupportsLabel {
			caps = append(caps, "labels")
		}
		capStr := "-"
		if len(caps) > 0 {
			capStr = strings.Join(caps, "+")
		}
		fmt.Fprintf(&b, "- %s - %s - %s\n", c.DisplayName, c.Slug, capStr)
	}
	return b.String()
}

func toolListTaxRules(ctx context.Context, r *Resolver, cid uuid.UUID) string {
	rules, err := r.TaxSvc.ListRulesByChannelHydrated(ctx, cid)
	if err != nil {
		return "Could not load tax rules."
	}
	if len(rules) == 0 {
		return "No tax rules for this channel."
	}
	var b strings.Builder
	b.WriteString("Tax rules (name - state - priority):\n")
	for _, rl := range rules {
		state := "active"
		if !rl.IsActive {
			state = "inactive"
		}
		fmt.Fprintf(&b, "- %s - %s - priority %d\n", rl.Name, state, rl.Priority)
	}
	return b.String()
}
