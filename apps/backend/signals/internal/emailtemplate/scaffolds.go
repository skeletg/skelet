package emailtemplate

// DefaultSubject + DefaultBlocks give an uncustomized template a sensible starting point in the admin editor (roughly mirroring the built-in design), so an operator edits from a working base rather than a blank canvas - Saving then creates the row; resetting deletes it (reverting to the built-in)

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
	case KeySignupConfirmation:
		return "Confirm your subscription"
	case KeyAccountExists:
		return "You already have an account"
	case KeyCartOffer:
		return "{{store.name}} prepared an offer for you"
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
	case KeySignupConfirmation:
		return []Block{
			{Type: "heading", Text: "Confirm your subscription"},
			{Type: "text", Content: "Please confirm you'd like to receive emails from {{store.name}}. This link expires in {{confirm.expiresIn}} - if you didn't sign up, ignore this email and you won't be subscribed."},
			{Type: "button", Label: "Confirm my subscription", URL: "{{confirm.link}}"},
		}
	case KeyAccountExists:
		return []Block{
			{Type: "heading", Text: "You already have an account"},
			{Type: "text", Content: "Someone just tried to create a {{store.name}} account with this email address, but you already have one. If this wasn't you, ignore this email - nothing about your account has changed."},
			{Type: "button", Label: "Sign in", URL: "{{signin.link}}"},
			{Type: "text", Content: "Forgotten your password? You can reset it here: {{forgot.link}}"},
		}
	case KeyCartOffer:
		return []Block{
			{Type: "heading", Text: "An offer, just for you"},
			{Type: "text", Content: "Hi {{customer.firstName}}, {{store.name}} has put together an order for you. {{offer.message}}"},
			{Type: "button", Label: "View your offer", URL: "{{offer.link}}"},
			{Type: "text", Content: "The offer is open until {{offer.expiresAt}}."},
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
