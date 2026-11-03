package shoptools

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/ai"
	"github.com/skeletg/skelet/apps/backend/core/internal/cart"
)

const maxLinesPerUpdate = 50

const cartDoc = `query AgentCartRead($id: ID!) {
  cart(id: $id) {
    id subtotal totalDiscount
    lines { sku name quantity unitPrice totalPrice availableQuantity currentUnitPrice }
    appliedCodes { code publicLabel subtotalDiscount freeShipping invalidatedReason requiresCode }
  }
}`

const (
	addDoc    = `mutation AgentAdd($cartId: ID!, $sku: String!, $qty: Float!) { addToCart(cartId: $cartId, sku: $sku, quantity: $qty) { id } }`
	updateDoc = `mutation AgentUpdate($cartId: ID!, $lineId: ID!, $qty: Float!) { updateCartLine(cartId: $cartId, lineId: $lineId, quantity: $qty) { id } }`
	removeDoc = `mutation AgentRemove($cartId: ID!, $lineId: ID!) { removeFromCart(cartId: $cartId, lineId: $lineId) { id } }`
	couponDoc = `mutation AgentCoupon($cartId: ID!, $code: String!) { applyCoupon(cartId: $cartId, code: $code) { id } }`
)

func cartIDProp() map[string]any { return str("The cart id update_cart answered with.") }

func getCartTool() tool {
	return tool{
		def: ai.Tool{
			Name:        "get_cart",
			Description: "What's in a cart this conversation built: its lines, discount codes and subtotal.",
			Schema:      schema(map[string]any{"cart_id": cartIDProp()}, "cart_id"),
			ReadOnly:    true, Idempotent: true,
		},
		run: func(ctx context.Context, c call) (string, error) {
			ct, err := agentCart(ctx, c, c.a.CartID)
			if err != nil {
				return "", err
			}
			return renderCart(ctx, c, ct.ID, nil)
		},
	}
}

func updateCartTool() tool {
	return tool{
		def: ai.Tool{
			Name: "update_cart",
			Description: "Build the shopper's cart. Leave cart_id out to start a new cart, and pass the cart_id it answers with on every later call. " +
				"Each line sets a SKU to an exact quantity - 0 takes it out - so repeating a call changes nothing. Use the SKUs get_product says go in the cart.",
			Schema: schema(map[string]any{
				"cart_id": str("The cart to change. Omit to start a new one."),
				"lines": map[string]any{
					"type":        "array",
					"maxItems":    maxLinesPerUpdate,
					"description": "The SKUs to set, each to the quantity the cart should hold.",
					"items": map[string]any{
						"type":                 "object",
						"additionalProperties": false,
						"required":             []string{"sku", "quantity"},
						"properties": map[string]any{
							"sku":      str("A SKU get_product says goes in the cart."),
							"quantity": map[string]any{"type": "number", "minimum": 0, "description": "How many the cart should hold - 0 removes the line."},
						},
					},
				},
				"coupon": str("A discount code the shopper gave you."),
			}),
			Idempotent: true,
		},
		run: runUpdateCart,
	}
}

func runUpdateCart(ctx context.Context, c call) (string, error) {
	a := c.a
	if len(a.Lines) > maxLinesPerUpdate {
		return "", refusal(fmt.Sprintf("Send at most %d lines at a time.", maxLinesPerUpdate))
	}
	var ct cart.Cart
	var err error
	created := a.CartID == ""
	if created {
		if len(a.Lines) == 0 && strings.TrimSpace(a.Coupon) == "" {
			return "", refusal("Pass the lines to put in the new cart.")
		}
		ct, err = c.deps.Carts.CreateAgentCart(ctx, c.ch.ID)
	} else {
		ct, err = agentCart(ctx, c, a.CartID)
	}
	if err != nil {
		return "", err
	}
	id := ct.ID.String()
	plain := map[string]cart.CartLine{}
	for _, l := range ct.Lines {
		if len(l.Options) == 0 && len(l.Components) == 0 {
			plain[l.ProductSku] = l
		}
	}
	wanted := map[string]float64{}
	var order []string
	for _, l := range a.Lines {
		sku := strings.TrimSpace(l.SKU)
		if sku == "" || l.Quantity == nil || *l.Quantity < 0 {
			return "", refusal("Every line needs a sku and a quantity of 0 or more.")
		}
		if _, seen := wanted[sku]; !seen {
			order = append(order, sku)
		}
		wanted[sku] = *l.Quantity
	}
	var notes []string
	for _, sku := range order {
		qty := wanted[sku]
		existing, has := plain[sku]
		var doc string
		vars := map[string]any{"cartId": id}
		switch {
		case has && qty == 0:
			doc, vars["lineId"] = removeDoc, existing.ID.String()
		case has && qty != existing.Quantity:
			doc, vars["lineId"], vars["qty"] = updateDoc, existing.ID.String(), qty
		case !has && qty > 0:
			doc, vars["sku"], vars["qty"] = addDoc, sku, qty
		default:
			continue
		}
		if err := c.deps.GraphQL.Do(ctx, doc, vars, nil); err != nil {
			var gql *Error
			if !errors.As(err, &gql) {
				return "", err
			}
			notes = append(notes, fmt.Sprintf("%s wasn't changed: %s", sku, lineRefusal(gql)))
		}
	}
	if code := strings.TrimSpace(a.Coupon); code != "" {
		if err := c.deps.GraphQL.Do(ctx, couponDoc, map[string]any{"cartId": id, "code": code}, nil); err != nil {
			var gql *Error
			if !errors.As(err, &gql) {
				return "", err
			}
			notes = append(notes, fmt.Sprintf("Code %s wasn't applied: %s", code, couponRefusal(gql)))
		}
	}
	if created {
		notes = append([]string{fmt.Sprintf("New cart - pass cart_id %s on every later cart call.", id)}, notes...)
	}
	return renderCart(ctx, c, ct.ID, notes)
}

func lineRefusal(e *Error) string {
	switch e.Code {
	case "PRODUCT_OPTIONS_REQUIRED", "PRODUCT_NOT_PURCHASABLE", "PRODUCT_NOT_BUNDLE":
		return "it needs choices made on the product page, or it's the parent of variants - get_product says which SKU to use"
	case "PRODUCT_NOT_PUBLISHED", "PRODUCT_NOT_FOUND", "NOT_FOUND":
		return "there's no such product on this store"
	}
	return e.Message
}

func couponRefusal(e *Error) string {
	switch {
	case e.Code == "PROMOTION_CODE_NOT_FOUND":
		return "the store doesn't know that code"
	case e.Code == "PROMOTION_CODE_ON_AUTO_APPLY_RULE":
		return "that discount applies by itself, no code needed"
	case strings.HasSuffix(e.Code, "_INACTIVE"):
		return "that code isn't active"
	case strings.HasSuffix(e.Code, "_MAX_USES"):
		return "that code has been used up"
	case strings.HasSuffix(e.Code, "_NOT_YET_VALID"):
		return "that code isn't valid yet"
	case strings.HasSuffix(e.Code, "_EXPIRED"):
		return "that code has expired"
	case strings.Contains(e.Code, "PER_CUSTOMER_LIMIT"):
		return "that code was already used"
	}
	return e.Message
}

func renderCart(ctx context.Context, c call, id uuid.UUID, notes []string) (string, error) {
	var out struct {
		Cart *struct {
			ID            string `json:"id"`
			Subtotal      int64  `json:"subtotal"`
			TotalDiscount int64  `json:"totalDiscount"`
			Lines         []struct {
				SKU               string   `json:"sku"`
				Name              string   `json:"name"`
				Quantity          float64  `json:"quantity"`
				UnitPrice         int64    `json:"unitPrice"`
				TotalPrice        int64    `json:"totalPrice"`
				AvailableQuantity *float64 `json:"availableQuantity"`
				CurrentUnitPrice  *int64   `json:"currentUnitPrice"`
			} `json:"lines"`
			AppliedCodes []struct {
				Code              string `json:"code"`
				PublicLabel       string `json:"publicLabel"`
				SubtotalDiscount  int64  `json:"subtotalDiscount"`
				FreeShipping      bool   `json:"freeShipping"`
				InvalidatedReason string `json:"invalidatedReason"`
				RequiresCode      bool   `json:"requiresCode"`
			} `json:"appliedCodes"`
		} `json:"cart"`
	}
	if err := c.deps.GraphQL.Do(ctx, cartDoc, map[string]any{"id": id.String()}, &out); err != nil {
		return "", err
	}
	if out.Cart == nil {
		return "", refusal("That cart is gone - leave cart_id out to start a new one.")
	}
	cur := c.ch.CurrencyCode
	var b strings.Builder
	for _, n := range notes {
		b.WriteString(n + "\n")
	}
	fmt.Fprintf(&b, "Cart %s:\n", out.Cart.ID)
	if len(out.Cart.Lines) == 0 {
		b.WriteString("(empty)\n")
	}
	for _, l := range out.Cart.Lines {
		fmt.Fprintf(&b, "- %s [sku %s] %g x %s = %s", l.Name, l.SKU, l.Quantity, price(l.UnitPrice, cur), price(l.TotalPrice, cur))
		if l.AvailableQuantity != nil && *l.AvailableQuantity < l.Quantity {
			fmt.Fprintf(&b, " - only %g available", *l.AvailableQuantity)
		}
		if l.CurrentUnitPrice != nil && *l.CurrentUnitPrice != l.UnitPrice {
			fmt.Fprintf(&b, " - now %s each", price(*l.CurrentUnitPrice, cur))
		}
		b.WriteString("\n")
	}
	for _, code := range out.Cart.AppliedCodes {
		label := code.Code
		if !code.RequiresCode || label == "" {
			label = code.PublicLabel
		}
		fmt.Fprintf(&b, "Discount %s", label)
		if code.SubtotalDiscount > 0 {
			fmt.Fprintf(&b, ": -%s", price(code.SubtotalDiscount, cur))
		}
		if code.FreeShipping {
			b.WriteString(", free shipping")
		}
		if code.InvalidatedReason != "" {
			fmt.Fprintf(&b, " (no longer applies: %s)", code.InvalidatedReason)
		}
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "Subtotal %s", price(out.Cart.Subtotal, cur))
	if out.Cart.TotalDiscount > 0 {
		fmt.Fprintf(&b, ", discounts -%s", price(out.Cart.TotalDiscount, cur))
	}
	b.WriteString(". Shipping and tax come at checkout - start_checkout when the shopper is ready.\n")
	return b.String(), nil
}

func checkoutTool() tool {
	return tool{
		def: ai.Tool{
			Name: "start_checkout",
			Description: "Finish up: closes the cart and gives a link for the shopper to open. It puts the cart in their own cart on the store, " +
				"where they review it, enter their details and pay. The cart can't change after this - asking again gives the same link.",
			Schema:     schema(map[string]any{"cart_id": cartIDProp()}, "cart_id"),
			Idempotent: true,
		},
		run: runCheckout,
	}
}

func runCheckout(ctx context.Context, c call) (string, error) {
	id, err := uuid.Parse(strings.TrimSpace(c.a.CartID))
	if err != nil {
		return "", refusal("cart_id takes the id update_cart answered with.")
	}
	ct, err := c.deps.Carts.StartHandoff(ctx, id, c.ch.ID)
	switch {
	case errors.Is(err, cart.ErrHandoffEmpty):
		return "", refusal("The cart is empty - put something in it with update_cart first.")
	case errors.Is(err, cart.ErrHandoffExpired):
		return "", refusal("That cart's checkout link has expired - build a new cart with update_cart.")
	case err != nil:
		return "", cartRefusal(err)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Checkout link for the shopper, valid until %s:\n%s\n\nIt puts these in their cart on the store:\n",
		ct.Handoff.ExpiresAt.Format("2 Jan 2006"), link(c.ch, cart.HandoffPath+ct.Handoff.Token))
	names := productNames(ctx, c, ct.Lines)
	for _, l := range ct.Lines {
		fmt.Fprintf(&b, "- %s [sku %s] x %g\n", cmp.Or(names[l.ProductSku], l.ProductName), l.ProductSku, l.Quantity)
	}
	b.WriteString("They'll see the final prices, shipping and tax there before paying. This cart is closed now - start a new one with update_cart if they want something else.\n")
	return b.String(), nil
}

const namesDoc = `query AgentNames($skus: [String!]!) { productsBySkus(skus: $skus) { sku name } }`

// The closed cart no longer reads through the front API, so its lines take their names from the catalog - best effort, the SKU is what counts
func productNames(ctx context.Context, c call, lines []cart.CartLine) map[string]string {
	skus := make([]string, len(lines))
	for i, l := range lines {
		skus[i] = l.ProductSku
	}
	var out struct {
		ProductsBySkus []struct{ SKU, Name string } `json:"productsBySkus"`
	}
	names := map[string]string{}
	if err := c.deps.GraphQL.Do(ctx, namesDoc, map[string]any{"skus": skus}, &out); err != nil {
		return names
	}
	for _, p := range out.ProductsBySkus {
		names[p.SKU] = p.Name
	}
	return names
}

func agentCart(ctx context.Context, c call, raw string) (cart.Cart, error) {
	id, err := uuid.Parse(strings.TrimSpace(raw))
	if err != nil {
		return cart.Cart{}, refusal("cart_id takes the id update_cart answered with.")
	}
	ct, err := c.deps.Carts.AgentCart(ctx, id, c.ch.ID)
	if err != nil {
		return cart.Cart{}, cartRefusal(err)
	}
	return ct, nil
}

func cartRefusal(err error) error {
	switch {
	case errors.Is(err, cart.ErrCartForbidden):
		return refusal("There's no cart with that id here - leave cart_id out of update_cart to start a new one.")
	case errors.Is(err, cart.ErrCartNotActive):
		return refusal("That cart went to the shopper with start_checkout, so it can't change - leave cart_id out of update_cart to start a new one.")
	}
	return err
}
