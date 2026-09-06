package cart

import (
	"github.com/skeletg/skelet/apps/backend/internal/promotion"
)

func PromotionLineViews(c Cart) []promotion.CartLineView { // one projection for both callers - order placement and storefront cart hydration each had their own copy and drifted into pricing the same cart differently; catalog facts (kind, categories, attribute values) stay out because promotion.Service.EvaluateCart resolves them from the SKUs
	out := make([]promotion.CartLineView, len(c.Lines))
	for i, l := range c.Lines {
		out[i] = promotion.CartLineView{
			LineID:         l.ID,
			ProductSKU:     l.ProductSku,
			ProductName:    l.ProductName,
			Quantity:       l.Quantity,
			UnitPriceCents: l.UnitPrice,
		}
	}
	return out
}
