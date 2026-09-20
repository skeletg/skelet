package admin

// AI-chat tool definitions and handlers for the operational domains: page-aware entity lookups plus orders, inventory,
// promotions and reviews. The shared registry, dispatch and schema helpers live in aichat_tools.go

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/internal/ai"
)

// entityLookupSignalTools returns the page-aware entity lookups (operational, not Signals).
func entityLookupSignalTools() []signalTool {
	return []signalTool{
		// ---- Page-aware entity lookups (operational, not Signals) ----
		{
			def: ai.Tool{
				Name:        "get_product",
				Description: "Look up a product by id (e.g. the one on the current product page): name, SKU, kind, slug, default price.",
				Schema: objSchema(map[string]any{
					"product_id": map[string]any{"type": "string", "description": "Product UUID (from the current page context)."},
				}, "product_id"),
			},
			perm: func(p digestPerms) bool { return p.catalog },
			run: func(ctx context.Context, r *Resolver, _ uuid.UUID, _ time.Time, _ int, a toolArgs) string {
				return toolProduct(ctx, r, a)
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
			perm: func(p digestPerms) bool { return p.orders },
			run: func(ctx context.Context, r *Resolver, cid uuid.UUID, _ time.Time, _ int, a toolArgs) string {
				return toolOrder(ctx, r, cid, a)
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
			perm: func(p digestPerms) bool { return p.customers }, // the commerce record, so it stays with the suite off
			run: func(ctx context.Context, r *Resolver, cid uuid.UUID, _ time.Time, _ int, a toolArgs) string {
				return toolCustomer(ctx, r, cid, a)
			},
		},
	}
}

// opsSignalTools returns the orders / inventory / promotions / reviews tools.
func opsSignalTools() []signalTool {
	return []signalTool{
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
			perm: func(p digestPerms) bool { return p.orders },
			run: func(ctx context.Context, r *Resolver, cid uuid.UUID, _ time.Time, _ int, a toolArgs) string {
				return toolSearchOrders(ctx, r, cid, a)
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
			perm: func(p digestPerms) bool { return p.inventory },
			run: func(ctx context.Context, r *Resolver, cid uuid.UUID, _ time.Time, _ int, a toolArgs) string {
				return toolStock(ctx, r, cid, a)
			},
		},
		{
			def: ai.Tool{
				Name:        "list_low_stock",
				Description: "Items at or below their low-stock threshold (SKU, quantity vs threshold, location).",
				Schema:      windowSchema(map[string]any{"limit": limitProp("Max items to return (1-50). Default 20.", 50)}),
			},
			perm: func(p digestPerms) bool { return p.inventory },
			run: func(ctx context.Context, r *Resolver, _ uuid.UUID, _ time.Time, _ int, a toolArgs) string {
				return toolLowStock(ctx, r, a)
			},
		},
		// ---- Promotions (promotions.view) ----
		{
			def: ai.Tool{
				Name:        "list_promotions",
				Description: "List the store's promotion rules (name, whether a code is required, times used).",
				Schema:      objSchema(map[string]any{}),
			},
			perm: func(p digestPerms) bool { return p.promotions },
			run: func(ctx context.Context, r *Resolver, _ uuid.UUID, _ time.Time, _ int, _ toolArgs) string {
				return toolListPromotions(ctx, r)
			},
		},
		{
			def: ai.Tool{
				Name:        "get_promotion_performance",
				Description: "Promotion performance over a window: total redemptions, discount given, unique customers, and a per-rule breakdown.",
				Schema:      windowSchema(nil),
			},
			perm: func(p digestPerms) bool { return p.promotions },
			run: func(ctx context.Context, r *Resolver, _ uuid.UUID, since time.Time, _ int, _ toolArgs) string {
				return toolPromotionPerformance(ctx, r, since)
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
			perm: func(p digestPerms) bool { return p.reviews },
			run: func(ctx context.Context, r *Resolver, _ uuid.UUID, _ time.Time, _ int, a toolArgs) string {
				return toolProductReviews(ctx, r, a)
			},
		},
	}
}

func toolProduct(ctx context.Context, r *Resolver, a toolArgs) string {
	pid, err := uuid.Parse(strings.TrimSpace(a.ProductID))
	if err != nil {
		return "Provide a valid 'product_id'."
	}
	p, err := r.CatalogSvc.GetProduct(ctx, pid, r.activeLocale(ctx))
	if err != nil {
		return "Could not load the product."
	}
	name := p.SKU
	if p.Content != nil && strings.TrimSpace(p.Content.Name) != "" {
		name = p.Content.Name
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Product: %s\nSKU: %s\nKind: %s\nURL: %s\n", name, p.SKU, p.Kind, p.URL)
	if p.DefaultPrice > 0 {
		fmt.Fprintf(&b, "Default price: %s\n", fmtMoney(p.DefaultPrice, p.DefaultCurrencyCode))
	}
	return b.String()
}

func toolOrder(ctx context.Context, r *Resolver, cid uuid.UUID, a toolArgs) string {
	oid, err := uuid.Parse(strings.TrimSpace(a.OrderID))
	if err != nil {
		return "Provide a valid 'order_id'."
	}
	o, err := r.OrderSvc.GetByID(ctx, oid)
	if err != nil {
		return "Could not load the order."
	}
	if o.ChannelID != cid {
		return "That order belongs to a different channel."
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Order %s - status %s, payment %s.\n", o.Number, o.Status, o.PaymentStatus)
	fmt.Fprintf(&b, "Total: %s. Customer: %s.\n", fmtMoney(o.TotalAmount, o.CurrencyCode), o.Email)
	if len(o.Lines) > 0 {
		fmt.Fprintf(&b, "%d line(s):\n", len(o.Lines))
		for i, l := range o.Lines {
			if i >= 10 {
				break
			}
			fmt.Fprintf(&b, "- %s (%s) x%g - %s\n", l.ProductName, l.ProductSku, l.Quantity, fmtMoney(l.TotalPrice, o.CurrencyCode))
		}
	}
	return b.String()
}

func toolCustomer(ctx context.Context, r *Resolver, cid uuid.UUID, a toolArgs) string {
	custID, err := uuid.Parse(strings.TrimSpace(a.CustomerID))
	if err != nil {
		return "Provide a valid 'customer_id'."
	}
	c, err := r.CustomerSvc.GetByID(ctx, custID)
	if err != nil {
		return "Could not load the customer."
	}
	if c.ChannelID != cid {
		return "That customer belongs to a different channel."
	}
	name := strings.TrimSpace(c.FirstName + " " + c.LastName)
	if name == "" {
		name = c.Email
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Customer: %s <%s>\n", name, c.Email)
	if c.Phone != nil && strings.TrimSpace(*c.Phone) != "" {
		fmt.Fprintf(&b, "Phone: %s\n", *c.Phone)
	}
	fmt.Fprintf(&b, "Active: %t, email verified: %t, joined %s.\n", c.IsActive, c.IsEmailVerified, c.CreatedAt.Format("2006-01-02"))
	if c.Note != nil && strings.TrimSpace(*c.Note) != "" {
		fmt.Fprintf(&b, "Admin note: %s\n", *c.Note)
	}
	return b.String()
}

// ---- orders / inventory / promotions / reviews formatters ----

func toolSearchOrders(ctx context.Context, r *Resolver, cid uuid.UUID, a toolArgs) string {
	all, err := r.OrderSvc.ListOrders(ctx)
	if err != nil {
		return "Could not load orders."
	}
	q := strings.ToLower(strings.TrimSpace(a.Query))
	status := strings.ToLower(strings.TrimSpace(a.Status))
	limit := clampLimit(a.Limit, 20, 50)
	var b strings.Builder
	n := 0
	for _, o := range all {
		if o.ChannelID != cid {
			continue
		}
		if status != "" && strings.ToLower(o.Status) != status {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(o.Number), q) && !strings.Contains(strings.ToLower(o.Email), q) {
			continue
		}
		if n == 0 {
			b.WriteString("Orders (number - status - total - customer):\n")
		}
		fmt.Fprintf(&b, "- %s - %s - %s - %s [id %s]\n", o.Number, o.Status, fmtMoney(o.TotalAmount, o.CurrencyCode), o.Email, o.ID)
		n++
		if n >= limit {
			break
		}
	}
	if n == 0 {
		return "No matching orders in this channel."
	}
	return b.String()
}

func toolStock(ctx context.Context, r *Resolver, cid uuid.UUID, a toolArgs) string {
	pid, err := uuid.Parse(strings.TrimSpace(a.ProductID))
	if err != nil {
		return "Provide a valid 'product_id'."
	}
	var b strings.Builder
	if sal, e := r.InventorySvc.Salable(ctx, pid, cid); e == nil {
		fmt.Fprintf(&b, "Salable in this channel: %g\n", sal)
	}
	item, err := r.InventorySvc.GetItemByProduct(ctx, pid)
	if err != nil {
		if b.Len() == 0 {
			return "No inventory tracked for that product."
		}
		return b.String()
	}
	fmt.Fprintf(&b, "On hand (all locations): %g (low-stock threshold %g).\n", item.Quantity, item.LowStockThreshold)
	if levels, e := r.InventorySvc.ListLevelsForItem(ctx, item.ID); e == nil && len(levels) > 0 {
		locName := map[uuid.UUID]string{}
		if locs, e2 := r.InventorySvc.ListLocations(ctx); e2 == nil {
			for _, l := range locs {
				locName[l.ID] = l.Name
			}
		}
		b.WriteString("By location:\n")
		for _, lv := range levels {
			name := locName[lv.LocationID]
			if name == "" {
				name = lv.LocationID.String()
			}
			fmt.Fprintf(&b, "- %s: %g on hand, %g reserved (%g salable)\n", name, lv.Quantity, lv.ReservedQuantity, lv.Quantity-lv.ReservedQuantity)
		}
	}
	return b.String()
}

func toolLowStock(ctx context.Context, r *Resolver, a toolArgs) string {
	limit := clampLimit(a.Limit, 20, 50)
	levels, err := r.InventorySvc.ListLowStockLevels(ctx, int32(limit))
	if err != nil {
		return "Could not load low-stock levels."
	}
	if len(levels) == 0 {
		return "No low-stock items."
	}
	var b strings.Builder
	b.WriteString("Low-stock items (SKU - qty / threshold - location):\n")
	for i, lv := range levels {
		if i >= limit {
			break
		}
		loc := "-"
		if lv.LocationName != nil && strings.TrimSpace(*lv.LocationName) != "" {
			loc = *lv.LocationName
		} else if lv.LocationID == nil {
			loc = "unstocked"
		}
		fmt.Fprintf(&b, "- %s - %g / %g - %s\n", lv.ProductSKU, lv.Quantity, lv.LowStockThreshold, loc)
	}
	return b.String()
}

func toolListPromotions(ctx context.Context, r *Resolver) string {
	rules, _, err := r.PromotionSvc.ListRules(ctx, 100, 0)
	if err != nil {
		return "Could not load promotions."
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

func toolPromotionPerformance(ctx context.Context, r *Resolver, since time.Time) string {
	now := time.Now()
	tot, err := r.PromotionSvc.AnalyticsTotals(ctx, since, now)
	if err != nil {
		return "Could not load promotion performance."
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Promotions in window: %d redemptions, %.2f discount given, %d unique customers (store-wide, in store currency).\n",
		tot.RedemptionCount, float64(tot.DiscountTotalCents)/100, tot.UniqueCustomers)
	if rows, e := r.PromotionSvc.AnalyticsByRule(ctx, since, now); e == nil && len(rows) > 0 {
		b.WriteString("By rule:\n")
		for i, rw := range rows {
			if i >= 15 {
				break
			}
			fmt.Fprintf(&b, "- %s: %d redemptions, %.2f discount, %.2f order revenue, %d customers\n",
				rw.RuleName, rw.RedemptionCount, float64(rw.DiscountTotalCents)/100, float64(rw.OrderRevenueCents)/100, rw.UniqueCustomers)
		}
	}
	return b.String()
}

func toolProductReviews(ctx context.Context, r *Resolver, a toolArgs) string {
	pid, err := uuid.Parse(strings.TrimSpace(a.ProductID))
	if err != nil {
		return "Provide a valid 'product_id'."
	}
	agg, err := r.ReviewSvc.GetAggregates(ctx, pid)
	if err != nil {
		return "Could not load reviews."
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
	if sum, e := r.ReviewSvc.GetProductSummary(ctx, pid); e == nil && strings.TrimSpace(sum) != "" {
		fmt.Fprintf(&b, "What customers say: %s\n", sum)
	}
	return b.String()
}
