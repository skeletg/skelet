package webhook

import (
	"github.com/skeletg/skelet/apps/backend/core/internal/backinstock"
	"github.com/skeletg/skelet/apps/backend/core/internal/blog"
	"github.com/skeletg/skelet/apps/backend/core/internal/cart"
	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/cms"
	"github.com/skeletg/skelet/apps/backend/core/internal/consent"
	"github.com/skeletg/skelet/apps/backend/core/internal/customer"
	"github.com/skeletg/skelet/apps/backend/core/internal/download"
	"github.com/skeletg/skelet/apps/backend/core/internal/giftcard"
	"github.com/skeletg/skelet/apps/backend/core/internal/inventory"
	"github.com/skeletg/skelet/apps/backend/core/internal/menu"
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
	"github.com/skeletg/skelet/apps/backend/core/internal/payment"
	"github.com/skeletg/skelet/apps/backend/core/internal/qna"
	"github.com/skeletg/skelet/apps/backend/core/internal/subscription"
	"github.com/skeletg/skelet/apps/backend/core/internal/urlrewrite"
	"github.com/skeletg/skelet/apps/backend/core/internal/webform"
	"github.com/skeletg/skelet/apps/backend/core/internal/wishlist"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/lib/ratelimit"
)

func init() {
	// Async events - post-commit notifications
	RegisterEvent(EventDefinition{
		Type:        customer.EventRegistered,
		Kind:        KindAsync,
		Name:        "Customer Registered",
		Description: "Fires after a new customer registration commits. Response is ignored (async).",
	})
	RegisterEvent(EventDefinition{
		Type:        consent.EventChanged,
		Kind:        KindAsync,
		Name:        "Consent Changed",
		Description: "Fires after a customer opts in or out of a consent kind - marketing_email, marketing_sms and so on. Payload: CustomerID, Kind, OptedIn, Source. Account-global, so per-channel webhooks never match it.",
	})
	RegisterEvent(EventDefinition{
		Type:        customer.EventUpdated,
		Kind:        KindAsync,
		Name:        "Customer Updated",
		Description: "Fires after an admin updates a customer. Response is ignored (async). Payload includes changedFields listing which fields were modified.",
	})
	RegisterEvent(EventDefinition{
		Type:        order.EventPlaced,
		Kind:        KindAsync,
		Name:        "Order Placed",
		Description: "Fires after a new order commits. Response is ignored (async).",
	})
	RegisterEvent(EventDefinition{
		Type:        order.EventStatusChanged,
		Kind:        KindAsync,
		Name:        "Order Status Changed",
		Description: "Fires after a non-placement order status transition commits. Payload includes orderId, fromStatus, and toStatus. Response is ignored (async).",
	})
	RegisterEvent(EventDefinition{
		Type:        order.EventCancelled,
		Kind:        KindAsync,
		Name:        "Order Cancelled",
		Description: "Fires after an order transitions to cancelled. More specific alternative to filtering order.status_changed on toStatus.",
	})
	RegisterEvent(EventDefinition{
		Type:        order.EventFulfilled,
		Kind:        KindAsync,
		Name:        "Order Fulfilled",
		Description: "Fires after an order transitions to fulfilled. Subscribe here for shipping notifications, accounting closeout, or post-fulfillment surveys.",
	})

	// Cart lifecycle - post-commit notifications off the cart bus - Useful for external CRMs / marketing tools that mirror cart state or drive their own recovery - Line-level events can be chatty on busy stores; subscribe only to what you need - (There is no cart.abandoned webhook - abandonment is a scanner-derived state, surfaced via the abandoned-cart flow, not a bus event.)
	RegisterEvent(EventDefinition{
		Type:        cart.EventLineAdded,
		Kind:        KindAsync,
		Name:        "Cart Item Added",
		Description: "Fires after a line is added to a cart. Payload includes cartId, channelId, customerId (null for guests), anonId, lineId, productSku, unitPrice, quantity. Response ignored (async).",
	})
	RegisterEvent(EventDefinition{
		Type:        cart.EventLineUpdated,
		Kind:        KindAsync,
		Name:        "Cart Item Updated",
		Description: "Fires after a cart line's quantity changes. Payload includes cartId, lineId, quantity. Response ignored (async).",
	})
	RegisterEvent(EventDefinition{
		Type:        cart.EventLineRemoved,
		Kind:        KindAsync,
		Name:        "Cart Item Removed",
		Description: "Fires after a line is removed from a cart. Payload includes cartId, lineId. Response ignored (async).",
	})
	RegisterEvent(EventDefinition{
		Type:        cart.EventCleared,
		Kind:        KindAsync,
		Name:        "Cart Cleared",
		Description: "Fires after every line is removed from a cart. Payload includes cartId, channelId, customerId, anonId. Response ignored (async).",
	})
	RegisterEvent(EventDefinition{
		Type:        cart.EventMerged,
		Kind:        KindAsync,
		Name:        "Cart Merged",
		Description: "Fires after a guest cart merges into a customer cart on login (guest→customer conversion). Payload includes customerCartId, guestCartId, customerId, channelId. Response ignored (async).",
	})

	// Customer post-commit lifecycle
	RegisterEvent(EventDefinition{
		Type:        customer.EventDeleted,
		Kind:        KindAsync,
		Name:        "Customer Deleted",
		Description: "Fires after an admin deletion commits. Payload includes the customer's last-known email and channel_id so downstream systems can purge.",
	})
	RegisterEvent(EventDefinition{
		Type:        customer.EventEmailVerified,
		Kind:        KindAsync,
		Name:        "Customer Email Verified",
		Description: "Fires after a customer confirms their email via the verification link. Useful for moving the customer to a marketing-opted-in list or granting a welcome incentive.",
	})

	// Return (RMA) lifecycle. order.return_received already drives inventory
	// restock internally; these are the earlier lifecycle steps for parity
	RegisterEvent(EventDefinition{
		Type:        order.EventReturnRequested,
		Kind:        KindAsync,
		Name:        "Order Return Requested",
		Description: "Fires after a customer/admin opens a return (RMA) against a shipped order. Payload includes orderId and returnId.",
	})
	RegisterEvent(EventDefinition{
		Type:        order.EventReturnApproved,
		Kind:        KindAsync,
		Name:        "Order Return Approved",
		Description: "Fires after a requested return is approved by a moderator/admin.",
	})
	RegisterEvent(EventDefinition{
		Type:        order.EventReturnRejected,
		Kind:        KindAsync,
		Name:        "Order Return Rejected",
		Description: "Fires after a requested/approved return is rejected (terminal).",
	})

	// Customer self-service account changes (storefront)
	RegisterEvent(EventDefinition{
		Type:        customer.EventProfileUpdated,
		Kind:        KindAsync,
		Name:        "Customer Profile Updated",
		Description: "Fires after a customer edits their own profile. Payload includes changedFields.",
	})
	RegisterEvent(EventDefinition{
		Type:        customer.EventPasswordChanged,
		Kind:        KindAsync,
		Name:        "Customer Password Changed",
		Description: "Fires after a logged-in customer changes their own password (distinct from the reset-token flow).",
	})
	RegisterEvent(EventDefinition{
		Type:        customer.EventAddressChanged,
		Kind:        KindAsync,
		Name:        "Customer Address Changed",
		Description: "Fires after a customer adds, updates, or deletes an address book entry. Payload includes the action.",
	})

	// Subscription lifecycle
	RegisterEvent(EventDefinition{
		Type:        subscription.EventCreated,
		Kind:        KindAsync,
		Name:        "Subscription Created",
		Description: "Fires after a subscription is started from a placed order.",
	})
	RegisterEvent(EventDefinition{
		Type:        subscription.EventCanceled,
		Kind:        KindAsync,
		Name:        "Subscription Cancelled",
		Description: "Fires after a subscription is cancelled (customer or admin).",
	})
	RegisterEvent(EventDefinition{
		Type:        subscription.EventPaused,
		Kind:        KindAsync,
		Name:        "Subscription Paused",
		Description: "Fires after an admin pauses a subscription (billing suspended until resumed).",
	})
	RegisterEvent(EventDefinition{
		Type:        subscription.EventResumed,
		Kind:        KindAsync,
		Name:        "Subscription Resumed",
		Description: "Fires after a paused subscription is reactivated.",
	})
	RegisterEvent(EventDefinition{
		Type:        subscription.EventInvoiced,
		Kind:        KindAsync,
		Name:        "Subscription Invoiced",
		Description: "Fires after a billing-period invoice is minted (one per period). Payload includes the amount + currency.",
	})

	// Digital delivery
	RegisterEvent(EventDefinition{
		Type:        download.EventAccessed,
		Kind:        KindAsync,
		Name:        "Download Accessed",
		Description: "Fires when a buyer redeems a download token (a file is served).",
	})

	// Catalog product CRUD
	RegisterEvent(EventDefinition{
		Type:        catalog.EventProductCreated,
		Kind:        KindAsync,
		Name:        "Product Created",
		Description: "Fires after a new product row commits. Typical subscribers: search index sync, CDN cache warmer, ERP catalog mirror.",
	})
	RegisterEvent(EventDefinition{
		Type:        catalog.EventProductUpdated,
		Kind:        KindAsync,
		Name:        "Product Updated",
		Description: "Fires after a product row update commits. Payload includes changedFields listing the top-level fields that were modified.",
	})
	RegisterEvent(EventDefinition{
		Type:        catalog.EventProductDeleted,
		Kind:        KindAsync,
		Name:        "Product Deleted",
		Description: "Fires after a product row delete commits. Use the snapshot fields in the payload - the row is gone by the time this fires.",
	})

	// Inventory threshold alerts
	RegisterEvent(EventDefinition{
		Type:        inventory.EventStockLow,
		Kind:        KindAsync,
		Name:        "Stock Low",
		Description: "Fires when a stock level transitions across the per-item low_stock_threshold (from above to at-or-below the threshold while still positive).",
	})
	RegisterEvent(EventDefinition{
		Type:        inventory.EventStockOut,
		Kind:        KindAsync,
		Name:        "Stock Out",
		Description: "Fires when a stock level reaches zero from a positive quantity.",
	})

	// Blog publish lifecycle
	RegisterEvent(EventDefinition{
		Type:        blog.EventPostPublished,
		Kind:        KindAsync,
		Name:        "Blog Post Published",
		Description: "Fires after a blog post moves to status=published - admin click, scheduler promotion, or any other path. Useful for static-site rebuild triggers.",
	})
	RegisterEvent(EventDefinition{
		Type:        blog.EventPostUnpublished,
		Kind:        KindAsync,
		Name:        "Blog Post Unpublished",
		Description: "Fires after a blog post moves out of status=published into draft or archived.",
	})

	// CMS publish lifecycle
	RegisterEvent(EventDefinition{
		Type:        cms.EventPagePublished,
		Kind:        KindAsync,
		Name:        "CMS Page Published",
		Description: "Fires after a CMS page transitions to is_published=true. Subscribe to invalidate caches or kick off a static rebuild.",
	})
	RegisterEvent(EventDefinition{
		Type:        cms.EventPageUnpublished,
		Kind:        KindAsync,
		Name:        "CMS Page Unpublished",
		Description: "Fires after a CMS page transitions back to is_published=false.",
	})

	// Payments - settlement lifecycle - Subscribe for accounting, reconciliation,
	// fraud tooling, or driving downstream fulfillment off capture
	RegisterEvent(EventDefinition{
		Type:        payment.EventAuthorized,
		Kind:        KindAsync,
		Name:        "Payment Authorized",
		Description: "Fires after a payment authorization succeeds (funds held, not yet captured). Payload includes orderId, provider, amount, currency.",
	})
	RegisterEvent(EventDefinition{
		Type:        payment.EventCaptured,
		Kind:        KindAsync,
		Name:        "Payment Captured",
		Description: "Fires after funds are captured (money moves). The usual trigger for accounting + fulfillment.",
	})
	RegisterEvent(EventDefinition{
		Type:        payment.EventRefunded,
		Kind:        KindAsync,
		Name:        "Payment Refunded",
		Description: "Fires after a full or partial refund is issued. Payload includes the refunded amount.",
	})
	RegisterEvent(EventDefinition{
		Type:        payment.EventVoided,
		Kind:        KindAsync,
		Name:        "Payment Voided",
		Description: "Fires after an uncaptured authorization is voided (the hold is released).",
	})

	// Order lifecycle - the events beyond placed/status/cancel/fulfill
	RegisterEvent(EventDefinition{
		Type:        order.EventPaymentStatusChanged,
		Kind:        KindAsync,
		Name:        "Order Payment Status Changed",
		Description: "Fires when an order's payment state transitions (e.g. pending → paid → refunded). Payload includes orderId, fromStatus, toStatus.",
	})
	RegisterEvent(EventDefinition{
		Type:        order.EventEdited,
		Kind:        KindAsync,
		Name:        "Order Edited",
		Description: "Fires after an admin edits a placed order (lines/quantities/prices). Subscribe to re-sync external systems that mirror the order.",
	})
	RegisterEvent(EventDefinition{
		Type:        order.EventFulfillmentCreated,
		Kind:        KindAsync,
		Name:        "Order Fulfillment Created",
		Description: "Fires when a (possibly partial) fulfillment/shipment is created for an order. Payload includes the fulfilled lines + tracking when set.",
	})
	RegisterEvent(EventDefinition{
		Type:        order.EventReturnReceived,
		Kind:        KindAsync,
		Name:        "Order Return Received",
		Description: "Fires when returned goods are marked received (the step after return_approved). Completes the RMA lifecycle.",
	})

	// Reviews - moderation lifecycle - Subscribe to sync ratings to external widgets or notify on new submissions - String literals (not review.Event* constants) because internal/review → internal/ai → internal/webhook would form an import cycle; these values mirror review/events.go exactly
	RegisterEvent(EventDefinition{
		Type:        "review.submitted",
		Kind:        KindAsync,
		Name:        "Review Submitted",
		Description: "Fires after a customer submits a product review (pre-moderation). Payload includes productId, rating, and the author.",
	})
	RegisterEvent(EventDefinition{
		Type:        "review.approved",
		Kind:        KindAsync,
		Name:        "Review Approved",
		Description: "Fires after a review is approved and becomes publicly visible.",
	})
	RegisterEvent(EventDefinition{
		Type:        "review.rejected",
		Kind:        KindAsync,
		Name:        "Review Rejected",
		Description: "Fires after a review is rejected in moderation.",
	})

	// Catalog structure - categories, collections, and per-store product listings
	RegisterEvent(EventDefinition{
		Type:        catalog.EventCategoryCreated,
		Kind:        KindAsync,
		Name:        "Category Created",
		Description: "Fires after a catalog category is created.",
	})
	RegisterEvent(EventDefinition{
		Type:        catalog.EventCategoryUpdated,
		Kind:        KindAsync,
		Name:        "Category Updated",
		Description: "Fires after a category is updated (name, parent, URL, etc.).",
	})
	RegisterEvent(EventDefinition{
		Type:        catalog.EventCategoryDeleted,
		Kind:        KindAsync,
		Name:        "Category Deleted",
		Description: "Fires after a category is deleted.",
	})
	RegisterEvent(EventDefinition{
		Type:        catalog.EventCollectionCreated,
		Kind:        KindAsync,
		Name:        "Collection Created",
		Description: "Fires after a collection is created.",
	})
	RegisterEvent(EventDefinition{
		Type:        catalog.EventCollectionUpdated,
		Kind:        KindAsync,
		Name:        "Collection Updated",
		Description: "Fires after a collection is updated.",
	})
	RegisterEvent(EventDefinition{
		Type:        catalog.EventCollectionDeleted,
		Kind:        KindAsync,
		Name:        "Collection Deleted",
		Description: "Fires after a collection is deleted.",
	})
	RegisterEvent(EventDefinition{
		Type:        catalog.EventCollectionProductsChanged,
		Kind:        KindAsync,
		Name:        "Collection Products Changed",
		Description: "Fires when the set of products in a collection changes (manual collections). Payload includes the collection id.",
	})
	RegisterEvent(EventDefinition{
		Type:        catalog.EventAttributeUpdated,
		Kind:        KindAsync,
		Name:        "Attribute Updated",
		Description: "Fires after an attribute, its values, its product-type assignments or its flags change. Payload includes attributeId, slug and changedFields.",
	})
	RegisterEvent(EventDefinition{
		Type:        catalog.EventAttributeDeleted,
		Kind:        KindAsync,
		Name:        "Attribute Deleted",
		Description: "Fires after an attribute is deleted, along with every value products held for it. Payload includes attributeId and slug.",
	})
	RegisterEvent(EventDefinition{
		Type:        catalog.EventProductListingChanged,
		Kind:        KindAsync,
		Name:        "Product Listing Changed",
		Description: "Fires when a product's per-store listing changes (published/unpublished or price override). Payload includes productId + channelId.",
	})

	// Blog - full post lifecycle (publish/unpublish already covered above)
	RegisterEvent(EventDefinition{
		Type:        blog.EventPostCreated,
		Kind:        KindAsync,
		Name:        "Blog Post Created",
		Description: "Fires after a blog post is created (draft or published).",
	})
	RegisterEvent(EventDefinition{
		Type:        blog.EventPostUpdated,
		Kind:        KindAsync,
		Name:        "Blog Post Updated",
		Description: "Fires after a blog post's metadata is updated.",
	})
	RegisterEvent(EventDefinition{
		Type:        blog.EventPostDeleted,
		Kind:        KindAsync,
		Name:        "Blog Post Deleted",
		Description: "Fires after a blog post is deleted.",
	})
	RegisterEvent(EventDefinition{
		Type:        blog.EventPostContentChanged,
		Kind:        KindAsync,
		Name:        "Blog Post Content Changed",
		Description: "Fires after a post's body/content changes. Can be chatty during editing - subscribe only if you mirror content.",
	})
	RegisterEvent(EventDefinition{
		Type:        blog.EventPostListingChanged,
		Kind:        KindAsync,
		Name:        "Blog Post Listing Changed",
		Description: "Fires when a post's per-store listing changes.",
	})

	// Inventory - raw stock movements - High volume on busy catalogs; subscribe only if you sync stock to an external
	// system (stock_low / stock_out above are the low-frequency alerting events)
	RegisterEvent(EventDefinition{
		Type:        inventory.EventStockChanged,
		Kind:        KindAsync,
		Name:        "Stock Changed",
		Description: "Fires on any salable-quantity change for a product at a location. Chatty - payload includes productId, locationId, and the new quantity.",
	})

	// Merchandising + storefront config
	RegisterEvent(EventDefinition{
		Type:        menu.EventMenuUpdated,
		Kind:        KindAsync,
		Name:        "Menu Updated",
		Description: "Fires after a storefront menu (navigation) is updated. Subscribe to bust a headless frontend's nav cache.",
	})
	RegisterEvent(EventDefinition{
		Type:        channel.EventLocalesChanged,
		Kind:        KindAsync,
		Name:        "Channel Locales Changed",
		Description: "Fires after a store's active locale set changes.",
	})
	RegisterEvent(EventDefinition{
		Type:        channel.EventUpdated,
		Kind:        KindAsync,
		Name:        "Channel Updated",
		Description: "Fires after a store's own fields change - name, branding, currencies, public URL. Subscribe to bust a headless frontend's nav cache.",
	})
	RegisterEvent(EventDefinition{
		Type:        channel.EventDeleted,
		Kind:        KindAsync,
		Name:        "Channel Deleted",
		Description: "Fires after a store/channel is deleted.",
	})

	// Web forms - lead capture - Subscribe to push submissions into a CRM
	RegisterEvent(EventDefinition{
		Type:        webform.EventFormSubmitted,
		Kind:        KindAsync,
		Name:        "Web Form Submitted",
		Description: "Fires after a storefront web form (contact, lead, custom) is submitted. Payload includes the form id + submitted fields.",
	})

	// Auth - security signals - Useful for SIEM / anomaly detection. login.failed
	// in particular drives brute-force alerting
	RegisterEvent(EventDefinition{
		Type:        auth.EventLoginSucceeded,
		Kind:        KindAsync,
		Name:        "Login Succeeded",
		Description: "Fires after a successful login. Payload includes the actor + IP; no credentials.",
	})
	RegisterEvent(EventDefinition{
		Type:        auth.EventLoginFailed,
		Kind:        KindAsync,
		Name:        "Login Failed",
		Description: "Fires after a failed login attempt. Subscribe for brute-force / credential-stuffing alerting.",
	})
	RegisterEvent(EventDefinition{
		Type:        auth.EventPasswordReset,
		Kind:        KindAsync,
		Name:        "Password Reset Completed",
		Description: "Fires after a password is successfully changed via the reset-token flow (distinct from customer.password_changed, the logged-in self-service change). Payload includes the subject, subjectId, email, and channelId. Subscribe to send a 'your password was changed' notice or revoke downstream sessions.",
	})

	// URL rewrites - canonical URL + redirect changes - Primary subscriber: a
	// CDN / edge-cache purge that evicts the old path when a URL moves
	RegisterEvent(EventDefinition{
		Type:        urlrewrite.EventChanged,
		Kind:        KindAsync,
		Name:        "URL Rewrite Changed",
		Description: "Fires after a url_rewrite row is created, updated, or deleted (a canonical URL moved or a redirect changed). Payload includes path, entityType, entityId, channelId (null = global), and deleted. Subscribe to purge the affected path from edge caches.",
	})

	// Additional post-commit events
	RegisterEvent(EventDefinition{
		Type:        cart.EventBundleAdded,
		Kind:        KindAsync,
		Name:        "Cart Bundle Added",
		Description: "Fires after a bundle is added to a cart (the bundle counterpart to cart.line.added). Payload includes cartId, the bundle SKU, and the picked components.",
	})
	RegisterEvent(EventDefinition{
		Type:        payment.EventProviderWebhook,
		Kind:        KindAsync,
		Name:        "Payment Provider Webhook Received",
		Description: "Fires when an inbound payment-provider callback (e.g. Stripe/Braintree/PayPal) is received and processed - useful for reconciliation of async/delayed settlement events.",
	})

	// Auth - session lifecycle beyond login - Useful for SIEM, device tracking,
	// and concurrent-session enforcement
	RegisterEvent(EventDefinition{
		Type:        auth.EventLogout,
		Kind:        KindAsync,
		Name:        "Logout",
		Description: "Fires after a user (admin or customer) logs out a single session. Payload includes subject, subjectId, and allDevices (false for a single-device logout).",
	})
	RegisterEvent(EventDefinition{
		Type:        auth.EventRefreshIssued,
		Kind:        KindAsync,
		Name:        "Refresh Token Issued",
		Description: "Fires when a refresh token is minted - on login and on every refresh rotation, for both admin and customer. Payload includes subject, subjectId, sessionId (admin only), ip, userAgent. Chatty; subscribe for session-count metrics or concurrent-session caps.",
	})
	RegisterEvent(EventDefinition{
		Type:        auth.EventRefreshRevoked,
		Kind:        KindAsync,
		Name:        "Refresh Token Revoked",
		Description: "Fires when a refresh token is revoked. Payload includes subject, subjectId, and reason (rotation | logout | logout_other_devices | revoke_all). Subscribe to cascade session revocation to dependent systems.",
	})

	// Catalog - bundle lifecycle - A bundle is a product (kind=BUNDLE); these
	// fire alongside the product.* events as a more specific signal
	RegisterEvent(EventDefinition{
		Type:        catalog.EventBundleCreated,
		Kind:        KindAsync,
		Name:        "Bundle Created",
		Description: "Fires after a bundle product is created. Payload includes productId and SKU.",
	})
	RegisterEvent(EventDefinition{
		Type:        catalog.EventBundleUpdated,
		Kind:        KindAsync,
		Name:        "Bundle Updated",
		Description: "Fires after a bundle product is updated. Payload includes productId, SKU, and changedFields.",
	})
	RegisterEvent(EventDefinition{
		Type:        catalog.EventBundleDeleted,
		Kind:        KindAsync,
		Name:        "Bundle Deleted",
		Description: "Fires after a bundle product is deleted. Payload includes productId and SKU.",
	})

	// Gift cards - balance lifecycle - Subscribe for accounting, fraud tooling,
	// or fulfilling issued cards to recipients
	RegisterEvent(EventDefinition{
		Type:        giftcard.EventIssued,
		Kind:        KindAsync,
		Name:        "Gift Card Issued",
		Description: "Fires after a gift card is minted - an admin issue, or one card per gift-card line on a placed order. Payload includes giftCardId, code, currency, amount (opening balance, cents), customerId, and orderId (set when issued from a purchase).",
	})
	RegisterEvent(EventDefinition{
		Type:        giftcard.EventRedeemed,
		Kind:        KindAsync,
		Name:        "Gift Card Redeemed",
		Description: "Fires after a gift card is spent via the standalone redeem path. Payload includes giftCardId, code, amount applied, and remaining balance. (Checkout-tender redemption applied inside the order transaction is not emitted here.)",
	})
	RegisterEvent(EventDefinition{
		Type:        giftcard.EventAdjusted,
		Kind:        KindAsync,
		Name:        "Gift Card Adjusted",
		Description: "Fires after an admin balance correction (top-up, goodwill, chargeback). Payload includes giftCardId, signed delta, new balance, and note.",
	})

	// Wishlist - save signals - Prime input for back-in-stock and price-drop
	// marketing automations
	RegisterEvent(EventDefinition{
		Type:        wishlist.EventItemAdded,
		Kind:        KindAsync,
		Name:        "Wishlist Item Added",
		Description: "Fires after a signed-in customer saves a product to their wishlist. Payload includes customerId and productId.",
	})
	RegisterEvent(EventDefinition{
		Type:        wishlist.EventItemRemoved,
		Kind:        KindAsync,
		Name:        "Wishlist Item Removed",
		Description: "Fires after a customer removes a product from their wishlist. Payload includes customerId and productId.",
	})

	// Product Q&A - shopper questions and operator answers
	RegisterEvent(EventDefinition{
		Type:        qna.EventQuestionSubmitted,
		Kind:        KindAsync,
		Name:        "Product Question Submitted",
		Description: "Fires after a shopper submits a product question (pending moderation). Payload includes questionId, productId, channelId, customerId, authorName, and the question text.",
	})
	RegisterEvent(EventDefinition{
		Type:        qna.EventQuestionAnswered,
		Kind:        KindAsync,
		Name:        "Product Question Answered",
		Description: "Fires after an operator answers a question (which publishes it to the storefront). Payload includes questionId, productId, channelId, answeredBy, and both the question and answer text.",
	})

	// Sync events - pre-commit hooks - Webhook response may reject or enrich the operation - Empty body or 2xx with no
	// JSON = accept, no changes Non-2xx or JSON {"reject": true, "reason": "..."} = abort the operation
	RegisterEvent(EventDefinition{
		Type: customer.EventRegistering,
		Kind: KindSync,
		Name: "Customer Registering (sync)",
		Description: "Fires before customer registration commits. Blocks the flow until response. " +
			`Response schema: {"reject": bool, "reason": string, "tags": [string]}. ` +
			"Returned tags are appended to the customer's registration tags.",
	})
	RegisterEvent(EventDefinition{
		Type: customer.EventUpdating,
		Kind: KindSync,
		Name: "Customer Updating (sync)",
		Description: "Fires before an admin customer update commits. Blocks the flow until response. " +
			`Response schema: {"reject": bool, "reason": string, "tags": [string]}. ` +
			"Returned tags are forwarded to the customer.updated async event.",
	})
	RegisterEvent(EventDefinition{
		Type: order.EventCheckoutValidating,
		Kind: KindSync,
		Name: "Checkout Validating (sync)",
		Description: "Fires when checkout is fully resolved (cart, addresses, payment provider, engine discount) but BEFORE the placement transaction opens - the preferred veto point for fraud screens, address checks and approval gates since a slow endpoint here never pins a DB transaction. " +
			`Response schema: {"reject": bool, "reason": string, "discountCents": int, "riskScore": int, "tags": [string]}. ` +
			"Enrichment is seeded onto the order.placing payload that follows; discounts accumulate, risk score takes the max, tags append.",
	})
	RegisterEvent(EventDefinition{
		Type: order.EventPlacing,
		Kind: KindSync,
		Name: "Order Placing (sync)",
		Description: "Fires before order placement commits. Blocks the flow until response. " +
			`Response schema: {"reject": bool, "reason": string, "discountCents": int, "riskScore": int, "tags": [string]}. ` +
			"Discounts accumulate across webhooks; risk score takes the max; tags append. " +
			"Tax/shipping/total are owned by the remote tax + shipping providers, not this webhook.",
	})
	RegisterEvent(EventDefinition{
		Type: order.EventTendering,
		Kind: KindSync,
		Name: "Order Tendering (sync)",
		Description: "Fires inside the placement transaction once the order exists and core's gift-card and store-credit tenders have been applied, carrying the balance still owed - the veto point for a credit line or pay-later arrangement, which refuses by rejecting. Payload: OrderID, OrderNumber, ChannelID, CustomerID (null for guests), Email, Provider, CurrencyCode, TotalCents, RemainingCents. " +
			`Response schema: {"reject": bool, "reason": string}. ` +
			"Published for every method with a balance, so match on Provider before acting. A slow endpoint here holds the placement transaction open - prefer checkout.validating for anything that only needs to say no.",
	})
	RegisterEvent(EventDefinition{
		Type: order.EventCancelling,
		Kind: KindSync,
		Name: "Order Cancelling (sync)",
		Description: "Fires before a cancel transition commits. Blocks the flow until response. " +
			`Response schema: {"reject": bool, "reason": string, "note": string}. ` +
			"Use to gate on refund success or to append a note onto the order_event audit row.",
	})
	RegisterEvent(EventDefinition{
		Type: order.EventFulfilling,
		Kind: KindSync,
		Name: "Order Fulfilling (sync)",
		Description: "Fires before a fulfilled transition commits. Blocks the flow until response. " +
			`Response schema: {"reject": bool, "reason": string, "carrier": string, "trackingNumber": string}. ` +
			"Carrier + trackingNumber, if returned, are appended to the order_event message.",
	})
	RegisterEvent(EventDefinition{
		Type: catalog.EventProductCreating,
		Kind: KindSync,
		Name: "Product Creating (sync)",
		Description: "Fires before a new product row inserts. Blocks the flow until response. " +
			`Response schema: {"reject": bool, "reason": string, "tags": [string]}. ` +
			"Returned tags are joined and stored under metadata.webhook_tags.",
	})
	RegisterEvent(EventDefinition{
		Type: catalog.EventProductUpdating,
		Kind: KindSync,
		Name: "Product Updating (sync)",
		Description: "Fires before a product update commits. Payload includes changedFields. " +
			`Response schema: {"reject": bool, "reason": string, "tags": [string]}. ` +
			"Returned tags are merged into metadata.webhook_tags.",
	})
	RegisterEvent(EventDefinition{
		Type: blog.EventPostPublishing,
		Kind: KindSync,
		Name: "Blog Post Publishing (sync)",
		Description: "Fires before a blog post moves to status=published - admin click or scheduler promotion. A scheduled post that is rejected stays scheduled and is asked again on the next run. " +
			`Response schema: {"reject": bool, "reason": string}.`,
	})
	RegisterEvent(EventDefinition{
		Type: cms.EventPagePublishing,
		Kind: KindSync,
		Name: "CMS Page Publishing (sync)",
		Description: "Fires before a CMS page transitions to is_published=true - admin click, bulk publish or scheduler promotion. A scheduled page that is rejected stays scheduled and is asked again on the next run. " +
			`Response schema: {"reject": bool, "reason": string}.`,
	})
	RegisterEvent(EventDefinition{
		Type: customer.EventDeleting,
		Kind: KindSync,
		Name: "Customer Deleting (sync)",
		Description: "Fires before an admin customer delete commits. Use to refuse the delete based on external state (open tickets, active subscriptions). " +
			`Response schema: {"reject": bool, "reason": string}.`,
	})

	// Cart pre-commit hooks (sync) - These block the cart operation on the webhook response - reject to refuse the change (fraud rules, restricted products, B2B approval, external reservation) - Cart line-add is a hot path; a slow/failing subscriber degrades add-to-cart, so subscribe deliberately
	RegisterEvent(EventDefinition{
		Type: cart.EventLineAdding,
		Kind: KindSync,
		Name: "Cart Item Adding (sync)",
		Description: "Fires before a simple line is added. Reject to block the add. " +
			`Response schema: {"reject": bool, "reason": string, "unitPriceCents": int, "quantity": number}. ` +
			"unitPriceCents/quantity override the line being added (external dynamic pricing, per-customer clamps); omit to keep the input.",
	})
	RegisterEvent(EventDefinition{
		Type: cart.EventBundleAdding,
		Kind: KindSync,
		Name: "Cart Bundle Adding (sync)",
		Description: "Fires before a bundle line is added. Reject to block the add. " +
			`Response schema: {"reject": bool, "reason": string, "unitPriceCents": int, "quantity": number}. ` +
			"unitPriceCents overrides the bundle's summed component price.",
	})
	RegisterEvent(EventDefinition{
		Type: cart.EventLineUpdating,
		Kind: KindSync,
		Name: "Cart Item Updating (sync)",
		Description: "Fires before a line quantity changes. Reject to block the change. " +
			`Response schema: {"reject": bool, "reason": string, "quantity": number}. ` +
			"quantity overrides the absolute new line quantity.",
	})
	RegisterEvent(EventDefinition{
		Type: cart.EventLineRemoving,
		Kind: KindSync,
		Name: "Cart Item Removing (sync)",
		Description: "Fires before a line is removed. Reject to block the removal. " +
			`Response schema: {"reject": bool, "reason": string}.`,
	})
	RegisterEvent(EventDefinition{
		Type: cart.EventMerging,
		Kind: KindSync,
		Name: "Cart Merging (sync)",
		Description: "Fires before a guest cart is merged into a customer's cart at login. " +
			`Response schema: {"reject": bool, "reason": string}.`,
	})

	// Review pre-moderation hook (sync) - Reject to refuse a submission (spam /
	// profanity filters) before it is ever stored
	RegisterEvent(EventDefinition{
		Type: "review.submitting",
		Kind: KindSync,
		Name: "Review Submitting (sync)",
		Description: "Fires before a product review is stored. Reject to refuse the submission. " +
			`Response schema: {"reject": bool, "reason": string, "status": "approved"|"rejected"|"pending", "tags": [string]}. ` +
			"status short-circuits manual moderation (external spam services); tags append and survive onto review.submitted. " +
			"String literal to avoid a webhook→review→ai import cycle; mirrors review/events.go.",
	})

	// Inventory pre-allocation hook (sync) - Reject to refuse a reservation (e.g. an external ERP owns stock truth) -
	// Fires during order placement - a rejecting/slow subscriber blocks checkout, so use with care
	RegisterEvent(EventDefinition{
		Type: inventory.EventAllocating,
		Kind: KindSync,
		Name: "Inventory Allocating (sync)",
		Description: "Fires before stock is reserved for an order line. Reject to refuse the allocation. " +
			`Response schema: {"reject": bool, "reason": string, "routing": {"<orderLineId>": "<locationId>"}}. ` +
			"Routed lines allocate from exactly the picked location (warehouse routing, store pickup); unrouted lines use the default algorithm.",
	})

	// Auth pre-commit hooks (sync) - Fires for BOTH admin and customer logins; a non-2xx (or a JSON reject) response blocks the operation.
	// login.attempting is a hot path - a slow/failing subscriber degrades every login - so subscribe deliberately
	RegisterEvent(EventDefinition{
		Type: auth.EventLoginAttempting,
		Kind: KindSync,
		Name: "Login Attempting (sync)",
		Description: "Fires before credentials are verified, for both admin and customer logins. Reject to block the attempt (IP allow/deny lists, geo-anomaly rules, external rate limits). Payload includes subject (admin|customer), email, ip, userAgent, and channelId (null for admin). " +
			`Response schema: {"reject": bool, "reason": string}.`,
	})
	RegisterEvent(EventDefinition{
		Type: auth.EventPasswordResetReq,
		Kind: KindSync,
		Name: "Password Reset Requesting (sync)",
		Description: "Fires before a password-reset token is minted. Reject to refuse the reset (e.g. SSO-only accounts that must not reset via email). A rejection is applied silently - the caller sees the same response as an unknown email - to preserve the enumeration defense. " +
			`Response schema: {"reject": bool, "reason": string}.`,
	})

	// Back-in-stock demand signal
	RegisterEvent(EventDefinition{
		Type:        backinstock.EventRequested,
		Kind:        KindAsync,
		Name:        "Back-in-stock Requested",
		Description: "Fires when a shopper subscribes to a restock alert for a product. Payload: ChannelID, ProductID, SKU, Email, CustomerID (null for guests).",
	})

	// Subscription dunning - one event per failed renewal charge
	RegisterEvent(EventDefinition{
		Type:        subscription.EventPaymentFailed,
		Kind:        KindAsync,
		Name:        "Subscription Payment Failed",
		Description: "Fires after a renewal charge fails. Attempt counts from 1; NextRetryAt is the next dunning step, null when the ladder is exhausted and WillCancel is true (the subscription was cancelled).",
	})

	// Ops signal: a client tripped the request throttle
	RegisterEvent(EventDefinition{
		Type:        ratelimit.EventThrottleTripped,
		Kind:        KindAsync,
		Name:        "Rate Limit Tripped",
		Description: "Fires when an IP exhausts a request budget on the front API. Payload: IP, Surface, Count, Budget, ChannelID. Feed it to a WAF or alerting.",
	})
}
