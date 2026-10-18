package signalsapi

// The vocabulary core and the suite agree on for Pulse: the moments a flow starts on, the keys its payload carries, and the merge tags the composer offers
// Core raises the moments and the suite runs the flows, so neither owns these alone

// The triggers a flow can be built on - core enrolls by naming one, the suite dispatches on it
const (
	TriggerOrderPlaced        = "order_placed"
	TriggerOrderShipped       = "order_shipped"
	TriggerOrderCancelled     = "order_cancelled"
	TriggerPaymentRefunded    = "payment_refunded"
	TriggerCustomerRegistered = "customer_registered"
	TriggerCartAbandoned      = "cart_abandoned"
	TriggerBrowseAbandoned    = "browse_abandoned"
	TriggerEmailOpened        = "email_opened"
	TriggerEmailClicked       = "email_clicked"
	TriggerSegmentEntered     = "segment_entered"
	TriggerBackInStock        = "back_in_stock"
	TriggerFormSubmitted      = "form_submitted"
	// The account triggers carry a one-time link and run inside the enrolling call - the mail has to
	// go out in the request, not on the engine's next tick
	TriggerEmailVerification = "email_verification"
	TriggerPasswordReset     = "password_reset"
	// A signup-form subscriber's double opt-in link, addressed by email since no account exists yet
	TriggerSignupConfirmation = "signup_confirmation"
	// Mailed to an account's owner when someone tries to register with the address it already holds
	TriggerAccountExists = "account_exists"
	// An offer cart an admin sends - its link goes out while the admin waits, like the account links
	TriggerCartOffer = "cart_offer"
)

// FlowTriggerTypes is every trigger something fires - the composer offers these and nothing else
func FlowTriggerTypes() []string {
	return []string{
		TriggerOrderPlaced, TriggerOrderShipped, TriggerOrderCancelled, TriggerPaymentRefunded,
		TriggerEmailVerification, TriggerPasswordReset, TriggerSignupConfirmation, TriggerAccountExists, TriggerCartOffer,
		TriggerCustomerRegistered, TriggerCartAbandoned, TriggerBrowseAbandoned, TriggerBackInStock,
		TriggerFormSubmitted, TriggerEmailOpened, TriggerEmailClicked, TriggerSegmentEntered,
	}
}

// IsAuthTrigger names the one-time-link triggers that enroll synchronously
func IsAuthTrigger(t string) bool {
	return t == TriggerEmailVerification || t == TriggerPasswordReset || t == TriggerSignupConfirmation || t == TriggerAccountExists || t == TriggerCartOffer
}

// The keys an enrollment payload carries, read back by the run as it renders - what each trigger
// puts there is the moment's own: the order snapshot, the reset link, the product that came back
const (
	PayloadOrderID        = "order_id"
	PayloadOrderNumber    = "order_number"
	PayloadOrder          = "order" // the email.OrderSnapshot the order emails render from
	PayloadFulfillmentID  = "fulfillment_id"
	PayloadRefundRef      = "refund_ref"
	PayloadVerifyLink     = "verify_link"
	PayloadResetLink      = "reset_link"
	PayloadResetExpiresIn = "reset_expires_in"
	PayloadConfirmLink    = "confirm_link"
	PayloadConfirmExpires = "confirm_expires_in"
	PayloadSignInLink     = "signin_link"
	PayloadForgotLink     = "forgot_link"
	PayloadOfferLink      = "offer_link"
	PayloadOfferExpires   = "offer_expires"
	PayloadOfferMessage   = "offer_message"
	PayloadStoreName      = "store_name"
	PayloadFirstName      = "first_name"
	PayloadCartID         = "cart_id"
	PayloadPageKey        = "page_key"
	PayloadSKU            = "sku"
	PayloadProductName    = "product_name"
	PayloadProductImage   = "product_image"
	PayloadProductURL     = "page_key_url"
	PayloadSegmentSlug    = "segment_slug"
	PayloadFormSlug       = "form_slug"
)

// CampaignMergeTags drives the composer's tag palette for a broadcast
func CampaignMergeTags() []string {
	return []string{"store.name", "store.logoUrl", "storefront.url", "customer.firstName", "customer.email", "unsubscribe.link"}
}

// FlowMergeTags is everything a campaign offers plus the cart-recovery link and the browsed-product
// tags the abandonment flows render
func FlowMergeTags() []string {
	return append(CampaignMergeTags(), "cart.recoveryUrl", "product.name", "product.image", "product.url")
}
