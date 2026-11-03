package agenttools

// Agent tool definitions and handlers for the operational domains: page-aware entity lookups plus orders, inventory,
// promotions and reviews. The shared registry, dispatch and schema helpers live in tools.go

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/internal/ai"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/customer"
	"github.com/skeletg/skelet/apps/backend/core/internal/inventory"
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
	"github.com/skeletg/skelet/apps/backend/core/internal/promotion"
	"github.com/skeletg/skelet/apps/backend/core/lib/money"
)

// entityLookupTools returns the page-aware entity lookups (operational, not Signals).
func entityLookupTools() []tool {
	return []tool{
		// ---- Page-aware entity lookups (operational, not Signals) ----
		{
			def: ai.Tool{
				Name:        "get_product",
				Description: "Look up a product by id (e.g. the one on the current product page): name, SKU, kind, slug, default price.",
				Schema: objSchema(map[string]any{
					"product_id": map[string]any{"type": "string", "description": "Product UUID (from the current page context)."},
				}, "product_id"),
			},
			perm: func(p Perms) bool { return p.Catalog },
			run: func(ctx context.Context, deps *Deps, _ uuid.UUID, _ time.Time, _ int, a toolArgs) string {
				return toolProduct(ctx, deps, a)
			},
		},
		{
			def: ai.Tool{
				Name:        "get_order",
				Description: "Look up an order by id (e.g. the one on the current order page): number, status, payment status, total, customer, and line items.",
				Schema: objSchema(map[string]any{
					"order_id": map[string]any{"type": "string", "description": "Order UUID (from the current page context)."},
				}, "order_id"),
			},
			perm: func(p Perms) bool { return p.Orders },
			run: func(ctx context.Context, deps *Deps, cid uuid.UUID, _ time.Time, _ int, a toolArgs) string {
				return toolOrder(ctx, deps, cid, a)
			},
		},
		{
			def: ai.Tool{
				Name:        "get_customer",
				Description: "Look up a customer RECORD by id (name, email, phone, status, join date, admin note) - the operational record, distinct from get_customer_profile (behavioural analytics).",
				Schema: objSchema(map[string]any{
					"customer_id": map[string]any{"type": "string", "description": "Customer UUID (from the current page context)."},
				}, "customer_id"),
			},
			perm: func(p Perms) bool { return p.Customers }, // the commerce record, so it stays with the suite off
			run: func(ctx context.Context, deps *Deps, cid uuid.UUID, _ time.Time, _ int, a toolArgs) string {
				return toolCustomer(ctx, deps, cid, a)
			},
		},
	}
}

// opsTools returns the orders / inventory / promotions / reviews tools.
func opsTools() []tool {
	return []tool{
		// ---- Orders (orders.view) ----
		{
			def: ai.Tool{
				Name:        "search_orders",
				Description: "Search/list this channel's orders by order number or customer email, optionally filtered by status. Emits order ids for get_order.",
				Schema: windowSchema(map[string]any{
					"query":  map[string]any{"type": "string", "description": "Match on order number or customer email. Omit to list recent."},
					"status": map[string]any{"type": "string", "description": "Optional order status filter (e.g. pending, paid, fulfilled, cancelled)."},
					"limit":  limitProp("Max orders to return (1-50). Default 20.", 50),
				}),
			},
			perm: func(p Perms) bool { return p.Orders },
			run: func(ctx context.Context, deps *Deps, cid uuid.UUID, _ time.Time, _ int, a toolArgs) string {
				return toolSearchOrders(ctx, deps, cid, a)
			},
		},
		// ---- Inventory (inventory.view) ----
		{
			def: ai.Tool{
				Name:        "get_stock",
				Description: "Stock for a product by id: salable in this channel, total on-hand, and per-location on-hand/reserved.",
				Schema: objSchema(map[string]any{
					"product_id": map[string]any{"type": "string", "description": "Product UUID."},
				}, "product_id"),
			},
			perm: func(p Perms) bool { return p.Inventory },
			run: func(ctx context.Context, deps *Deps, cid uuid.UUID, _ time.Time, _ int, a toolArgs) string {
				return toolStock(ctx, deps, cid, a)
			},
		},
		{
			def: ai.Tool{
				Name:        "list_low_stock",
				Description: "Items at or below their low-stock threshold (SKU, quantity vs threshold, location).",
				Schema:      windowSchema(map[string]any{"limit": limitProp("Max items to return (1-50). Default 20.", 50)}),
			},
			perm: func(p Perms) bool { return p.Inventory },
			run: func(ctx context.Context, deps *Deps, cid uuid.UUID, _ time.Time, _ int, a toolArgs) string {
				return toolLowStock(ctx, deps, cid, a)
			},
		},
		// ---- Promotions (promotions.view) ----
		{
			def: ai.Tool{
				Name:        "list_promotions",
				Description: "List the store's promotion rules (name, whether a code is required, times used).",
				Schema:      objSchema(map[string]any{}),
			},
			perm: func(p Perms) bool { return p.Promotions },
			run: func(ctx context.Context, deps *Deps, cid uuid.UUID, _ time.Time, _ int, _ toolArgs) string {
				return toolListPromotions(ctx, deps, cid)
			},
		},
		{
			def: ai.Tool{
				Name:        "get_promotion_performance",
				Description: "Promotion performance over a window: total redemptions, discount given, unique customers, and a per-rule breakdown.",
				Schema:      windowSchema(nil),
			},
			perm: func(p Perms) bool { return p.Promotions },
			run: func(ctx context.Context, deps *Deps, cid uuid.UUID, since time.Time, _ int, _ toolArgs) string {
				return toolPromotionPerformance(ctx, deps, cid, since)
			},
		},
		// ---- Reviews (reviews.view) ----
		{
			def: ai.Tool{
				Name:        "get_product_reviews",
				Description: "Review aggregates for a product by id: review count, average rating, star distribution, and the AI 'what customers say' summary.",
				Schema: objSchema(map[string]any{
					"product_id": map[string]any{"type": "string", "description": "Product UUID."},
				}, "product_id"),
			},
			perm: func(p Perms) bool { return p.Reviews },
			run: func(ctx context.Context, deps *Deps, _ uuid.UUID, _ time.Time, _ int, a toolArgs) string {
				return toolProductReviews(ctx, deps, a)
			},
		},
	}
}

func toolProduct(ctx context.Context, deps *Deps, a toolArgs) string {
	pid, err := uuid.Parse(strings.TrimSpace(a.ProductID))
	if err != nil {
		return "Provide a valid 'product_id'."
	}
	p, err := deps.Catalog.GetProduct(ctx, pid, deps.activeLocale(ctx))
	if err != nil {
		return toolFailed(ctx, err, "Could not load the product.")
	}
	name := p.SKU
	if p.Content != nil && strings.TrimSpace(p.Content.Name) != "" {
		name = p.Content.Name
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Product: %s\nSKU: %s\nKind: %s\nURL: %s\n", name, p.SKU, p.Kind, p.URL)
	if p.DefaultPrice > 0 {
		fmt.Fprintf(&b, "Default price: %s\n", money.FormatCode(p.DefaultPrice, p.DefaultCurrencyCode))
	}
	return b.String()
}

func toolOrder(ctx context.Context, deps *Deps, cid uuid.UUID, a toolArgs) string {
	oid, err := uuid.Parse(strings.TrimSpace(a.OrderID))
	if err != nil {
		return "Provide a valid 'order_id'."
	}
	o, err := deps.Order.GetByID(ctx, oid)
	if errors.Is(err, order.ErrOrderNotFound) || err == nil && o.ChannelID != cid {
		return "No order with that id on this channel."
	}
	if err != nil {
		return toolFailed(ctx, err, "Could not load the order.")
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Order %s - status %s, payment %s.\n", o.Number, o.Status, o.PaymentStatus)
	fmt.Fprintf(&b, "Total: %s. Customer: %s.\n", money.FormatCode(o.TotalAmount, o.CurrencyCode), shopperEmail(ctx, o.Email))
	if len(o.Lines) > 0 {
		fmt.Fprintf(&b, "%d line(s):\n", len(o.Lines))
		for i, l := range o.Lines {
			if i >= 10 {
				break
			}
			fmt.Fprintf(&b, "- %s (%s) x%g - %s\n", l.ProductName, l.ProductSku, l.Quantity, money.FormatCode(l.TotalPrice, o.CurrencyCode))
		}
	}
	return b.String()
}

func toolCustomer(ctx context.Context, deps *Deps, cid uuid.UUID, a toolArgs) string {
	custID, err := uuid.Parse(strings.TrimSpace(a.CustomerID))
	if err != nil {
		return "Provide a valid 'customer_id'."
	}
	c, err := deps.Customer.GetByID(ctx, custID)
	if errors.Is(err, customer.ErrCustomerNotFound) || err == nil && c.ChannelID != cid {
		return "No customer with that id on this channel."
	}
	if err != nil {
		return toolFailed(ctx, err, "Could not load the customer.")
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Customer: %s, email %s\n", untrusted(strings.TrimSpace(c.FirstName+" "+c.LastName)), shopperEmail(ctx, c.Email))
	if c.Phone != nil && strings.TrimSpace(*c.Phone) != "" {
		fmt.Fprintf(&b, "Phone: %s\n", untrusted(*c.Phone))
	}
	fmt.Fprintf(&b, "Active: %t, email verified: %t, joined %s.\n", c.IsActive, c.IsEmailVerified, c.CreatedAt.Format("2006-01-02"))
	if c.Note != nil && strings.TrimSpace(*c.Note) != "" {
		fmt.Fprintf(&b, "Admin note: %s\n", *c.Note)
	}
	return b.String()
}

// ---- orders / inventory / promotions / reviews formatters ----

func toolSearchOrders(ctx context.Context, deps *Deps, cid uuid.UUID, a toolArgs) string {
	f := order.ListPageFilter{
		ChannelIDs: []uuid.UUID{cid},
		Search:     strings.TrimSpace(a.Query),
		Limit:      int32(clampLimit(a.Limit, 20, 50)),
	}
	if status := strings.ToLower(strings.TrimSpace(a.Status)); status != "" {
		f.Statuses = []string{status}
	}
	rows, _, err := deps.Order.ListOrdersPage(ctx, f)
	if err != nil {
		return toolFailed(ctx, err, "Could not load orders.")
	}
	if len(rows) == 0 {
		return "No matching orders in this channel."
	}
	var b strings.Builder
	b.WriteString("Orders (number - status - total - customer):\n")
	for _, o := range rows {
		fmt.Fprintf(&b, "- %s - %s - %s - %s [id %s]\n", o.Number, o.Status, money.FormatCode(o.TotalAmount, o.CurrencyCode), shopperEmail(ctx, o.Email), o.ID)
	}
	return b.String()
}

func toolStock(ctx context.Context, deps *Deps, cid uuid.UUID, a toolArgs) string {
	pid, err := uuid.Parse(strings.TrimSpace(a.ProductID))
	if err != nil {
		return "Provide a valid 'product_id'."
	}
	var b strings.Builder
	if sal, e := deps.Inventory.Salable(ctx, pid, cid); e == nil {
		fmt.Fprintf(&b, "Salable in this channel: %g\n", sal)
	}
	item, err := deps.Inventory.GetItemByProduct(ctx, pid)
	if err != nil {
		if b.Len() == 0 {
			return "No inventory tracked for that product."
		}
		return b.String()
	}
	fmt.Fprintf(&b, "Low-stock threshold: %g.\n", item.LowStockThreshold)
	locs, err := deps.Inventory.ListChannelLocations(ctx, cid)
	if err != nil {
		return toolFailed(ctx, err, b.String()+"Could not load this channel's locations.")
	}
	levels, err := deps.Inventory.ListLevelsForItem(ctx, item.ID)
	if err != nil {
		return toolFailed(ctx, err, b.String()+"Could not load stock levels.")
	}
	if own := channelLevels(levels, locs); len(own) > 0 {
		b.WriteString("By this channel's locations:\n")
		for _, lv := range own {
			fmt.Fprintf(&b, "- %s: %g on hand, %g reserved (%g salable)\n", lv.name, lv.Quantity, lv.ReservedQuantity, lv.Quantity-lv.ReservedQuantity)
		}
	}
	return b.String()
}

func toolLowStock(ctx context.Context, deps *Deps, cid uuid.UUID, a toolArgs) string {
	limit := clampLimit(a.Limit, 20, 50)
	levels, err := deps.Inventory.ListChannelLowStockLevels(ctx, cid, int32(limit))
	if err != nil {
		return toolFailed(ctx, err, "Could not load low-stock levels.")
	}
	if len(levels) == 0 {
		return "No low-stock items."
	}
	var b strings.Builder
	b.WriteString("Low-stock items (SKU - qty / threshold - location):\n")
	for _, lv := range levels {
		loc := "store-wide"
		if lv.LocationName != nil {
			loc = *lv.LocationName
		}
		fmt.Fprintf(&b, "- %s - %g / %g - %s\n", lv.ProductSKU, lv.Quantity, lv.LowStockThreshold, loc)
	}
	return b.String()
}

func toolListPromotions(ctx context.Context, deps *Deps, cid uuid.UUID) string {
	rules, _, err := deps.Promotion.ListRules(ctx, promotion.RuleFilter{ChannelIDs: []uuid.UUID{cid}, IncludeGlobal: true}, 100, 0)
	if err != nil {
		return toolFailed(ctx, err, "Could not load promotions.")
	}
	if len(rules) == 0 {
		return "No promotions defined."
	}
	var b strings.Builder
	b.WriteString("Promotions (name - trigger - uses):\n")
	for _, rl := range rules {
		trigger := "automatic"
		if rl.RequiresCode {
			trigger = "needs code"
		}
		fmt.Fprintf(&b, "- %s - %s - %d uses [id %s]\n", rl.Name, trigger, rl.UsedCount, rl.ID)
	}
	return b.String()
}

func toolPromotionPerformance(ctx context.Context, deps *Deps, cid uuid.UUID, since time.Time) string {
	now := time.Now()
	scope := promotion.AnalyticsScope{ChannelIDs: []uuid.UUID{cid}}
	tot, err := deps.Promotion.AnalyticsTotals(ctx, since, now, scope)
	if err != nil {
		return toolFailed(ctx, err, "Could not load promotion performance.")
	}
	code := ""
	if ch, ok := channel.FromContext(ctx); ok && ch != nil {
		code = ch.CurrencyCode
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Promotions in window: %d redemptions, %s discount given, %d unique customers (this channel).\n",
		tot.RedemptionCount, money.FormatCode(tot.DiscountTotalCents, code), tot.UniqueCustomers)
	if rows, e := deps.Promotion.AnalyticsByRule(ctx, since, now, scope); e != nil {
		skipped(ctx, e, "promotion performance by rule")
	} else if len(rows) > 0 {
		b.WriteString("By rule:\n")
		for _, rw := range rows[:min(len(rows), 15)] {
			fmt.Fprintf(&b, "- %s: %d redemptions, %s discount, %s order revenue, %d customers\n",
				rw.RuleName, rw.RedemptionCount, money.FormatCode(rw.DiscountTotalCents, code), money.FormatCode(rw.OrderRevenueCents, code), rw.UniqueCustomers)
		}
	}
	return b.String()
}

func toolProductReviews(ctx context.Context, deps *Deps, a toolArgs) string {
	pid, err := uuid.Parse(strings.TrimSpace(a.ProductID))
	if err != nil {
		return "Provide a valid 'product_id'."
	}
	agg, err := deps.Review.GetAggregates(ctx, pid)
	if err != nil {
		return toolFailed(ctx, err, "Could not load reviews.")
	}
	var b strings.Builder
	avg := "-"
	if agg.OverallAverage != nil {
		avg = fmt.Sprintf("%.1f", *agg.OverallAverage)
	}
	fmt.Fprintf(&b, "Reviews: %d, average %s/5.\n", agg.Count, avg)
	parts := make([]string, 0, 5)
	for i := 4; i >= 0; i-- {
		parts = append(parts, fmt.Sprintf("%d★=%d", i+1, agg.Distribution[i]))
	}
	fmt.Fprintf(&b, "Stars: %s\n", strings.Join(parts, ", "))
	if sum, e := deps.Review.GetProductSummary(ctx, pid); e == nil && strings.TrimSpace(sum) != "" {
		fmt.Fprintf(&b, "What customers say (an AI summary of their reviews): %s\n", untrusted(sum))
	}
	return b.String()
}

type namedLevel struct {
	inventory.Level
	name string
}

func channelLevels(levels []inventory.Level, locs []inventory.Location) []namedLevel {
	names := make(map[uuid.UUID]string, len(locs))
	for _, l := range locs {
		names[l.ID] = l.Name
	}
	out := make([]namedLevel, 0, len(levels))
	for _, lv := range levels {
		if name, ok := names[lv.LocationID]; ok {
			out = append(out, namedLevel{Level: lv, name: name})
		}
	}
	return out
}
