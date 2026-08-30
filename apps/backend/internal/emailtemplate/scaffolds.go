package emailtemplate

// DefaultSubject + DefaultBlocks give an uncustomized template a sensible
// starting point in the admin editor (roughly mirroring the built-in design),
// so an operator edits from a working base rather than a blank canvas - Saving
// then creates the row; resetting deletes it (reverting to the built-in)

// DefaultSubject returns the starter subject line for an email key
func DefaultSubject(key string) string {
	switch key {
	case KeyOrderConfirmation:
		return "Order {{order.number}} confirmed"
	case KeyOrderShipment:
		return "Order {{order.number}} has shipped"
	case KeyOrderCancellation:
		return "Order {{order.number}} cancelled"
	case KeyOrderRefund:
		return "Refund for order {{order.number}}"
	case KeyVerification:
		return "Verify your email address"
	case KeyPasswordReset:
		return "Reset your password"
	default:
		return ""
	}
}

// DefaultBlocks returns a starter block layout for an email key
func DefaultBlocks(key string) []Block {
	switch key {
	case KeyOrderConfirmation:
		return []Block{
			{Type: "heading", Text: "Thanks for your order, {{customer.firstName}}!"},
			{Type: "text", Content: "We've received order {{order.number}} and it's being processed."},
			{Type: "order_items"},
			{Type: "order_totals"},
			{Type: "shipping_address"},
			{Type: "button", Label: "View your order", URL: "{{storefront.url}}/account/orders"},
		}
	case KeyOrderShipment:
		return []Block{
			{Type: "heading", Text: "Your order is on the way"},
			{Type: "text", Content: "Hi {{customer.firstName}}, items from order {{order.number}} have shipped."},
			{Type: "tracking"},
			{Type: "order_items"},
		}
	case KeyOrderCancellation:
		return []Block{
			{Type: "heading", Text: "Order {{order.number}} cancelled"},
			{Type: "text", Content: "Hi {{customer.firstName}}, your order has been cancelled. If a payment was taken, any refund will follow separately."},
		}
	case KeyOrderRefund:
		return []Block{
			{Type: "heading", Text: "Refund issued"},
			{Type: "text", Content: "Hi {{customer.firstName}}, we've issued a refund for order {{order.number}}. It may take a few business days to appear."},
			{Type: "refund_amount"},
		}
	case KeyVerification:
		return []Block{
			{Type: "heading", Text: "Confirm your email"},
			{Type: "text", Content: "Welcome to {{store.name}}! Please confirm your email address to finish setting up your account."},
			{Type: "button", Label: "Verify my email", URL: "{{verify.link}}"},
		}
	case KeyPasswordReset:
		return []Block{
			{Type: "heading", Text: "Reset your password"},
			{Type: "text", Content: "We received a request to reset your {{store.name}} password. This link expires in {{reset.expiresIn}}."},
			{Type: "button", Label: "Reset password", URL: "{{reset.link}}"},
		}
	case KeyHeader:
		return []Block{{Type: "image", URL: "{{store.logoUrl}}", Alt: "{{store.name}}"}}
	case KeyFooter:
		return []Block{
			{Type: "divider"},
			{Type: "text", Content: "{{store.name}} - visit {{storefront.url}}"},
		}
	default:
		return nil
	}
}
