# Webhooks

Getting told when something happens in the store - and, for a few events, getting a say in
whether it happens at all.

A webhook is an HTTPS endpoint you own. Skelet POSTs a signed JSON payload to it when a
subscribed event fires. If instead of *observing* an event you want to *answer a question* -
what does shipping cost, how much tax is owed - you want a
[remote provider](remote-providers.md), not a webhook.

## Async and sync

This is the first decision, and the two behave very differently.

**Async** is the normal kind. It fires *after* the operation commits, cannot affect it, and
is retried on failure. Use it for "tell my ERP an order was placed."

**Sync** fires *during* the operation, before it commits. The emitting operation blocks
waiting for your response, and your reply can **reject the operation outright** or, for some
events, **modify it**. Sync webhooks are never retried - there is nothing to retry into,
since the operation has already been decided. Use it for "check this order against our fraud
list before it is accepted."

Sync webhooks are, in effect, business logic living on your server. Treat their latency as
checkout latency.

## Signing

Identical to the remote-provider scheme, so one verification helper covers both - only the
header names differ.

```
signature = HMAC_SHA256(secret, X-Webhook-Timestamp + "." + raw_request_body)
```

Sent as `X-Webhook-Signature-256: sha256=<hex>`, alongside `X-Webhook-Event`,
`X-Webhook-Timestamp` (unix seconds), `X-Webhook-Api-Version`, and any custom headers you
configured. The signing secret is shown once when the webhook is created.

The timestamp is inside the signed string specifically so you can enforce a replay window -
**five minutes is the recommended tolerance**, and without that check a captured delivery
stays replayable forever. Hash the raw bytes before parsing, and compare in constant time.
There's a worked example, in three languages, under *Verifying the signature* in the
[remote provider guide](remote-providers.md) - the only change is the header prefix.

Custom headers you set ride along, but they can never shadow the dispatcher's own; reserved
names (`Content-Type`, `X-Webhook-*`) are rejected when you save the webhook.

## Answering a sync webhook

Return **2xx** and the operation proceeds. Return anything else and what happens depends on
the webhook's **failure policy**:

- `fail_closed` (the default) - the emitting operation aborts. Your endpoint is a gate, so
  if it is down, orders stop.
- `fail_open` - the failure is logged and the operation continues. Your endpoint is
  advisory.

The distinction that matters: `fail_open` forgives *broken endpoints* - transport errors,
timeouts, non-2xx, malformed bodies. It does **not** forgive a deliberate veto. A 2xx
response whose body says `{"reject": true}` aborts the operation under **both** policies,
because that is you making a decision rather than your server failing.

Include a `reason` alongside it; it propagates back to the emitting service and into the
error the shopper or admin sees.

Some sync events also accept **modifications** in the response body - the fields differ per
event, because each event decides how to fold a response in. Cart line events take
`unitPriceCents` and `quantity` overrides, for instance. Events that don't implement this
treat sync delivery as a blocking notification: you can veto, but not edit.

A repeatedly failing endpoint trips a **circuit breaker**. While it is open, deliveries
short-circuit - which under `fail_closed` means the operation is refused with an error
telling you to fix the endpoint or switch the webhook to `fail_open`.

## Retries and the queue

Async deliveries that fail are retried with backoff. Every attempt - including retries -
writes its own **delivery record** with the request, the response, status and duration,
visible in the admin along with success/failure counts and the timestamps of the last
delivery, last success and last failure. When retries are exhausted the delivery lands in
the **dead-letter list**, where you can inspect and replay it after fixing the endpoint.

Each attempt has its own timeout: 5s for sync, 10s for async by default, overridable per
webhook up to 30s. Remember that a sync timeout is time the shopper spends waiting.

## Scope

A webhook is either **global** - it fires for every channel's events - or bound to one
channel, in which case it only fires for events carrying that channel. Events that aren't
channel-scoped only ever reach global webhooks.

Target URLs are checked against an SSRF guard, so internal and loopback addresses are
refused.

## Event reference

Event types are registered in code by the domain that emits them, and this table is
generated from that registry by `bin/codegen`.

**The two pools are disjoint.** Each event is either sync or async, and a webhook may only
subscribe to events of its own kind - a sync webhook subscribing to an async event is
rejected when you save it. So the `kind` column isn't a hint, it's which sort of webhook can
receive that event at all.

You can subscribe with **wildcards**: `*` for every event of the webhook's kind, or
`<prefix>.*` for a namespace, e.g. `order.*`. A wildcard is validated against your kind's
pool only, so `order.*` is valid on both a sync and an async webhook even though the
`order.` namespace contains some of each - each one receives only its own half.

**Enabled plugins emit their own**, declared in their manifest and registered at boot. Those
are runtime additions belonging to the plugin, so they are not in the core catalogue below -
the `b2b.*` and `company.*` events are the live example, and they arrive with the b2b plugin.
The event picker in Settings shows what your install actually has.

<!-- BEGIN GENERATED WEBHOOK EVENTS -->
<!-- Generated by cmd/docsref via bin/codegen. Edits here are overwritten. -->

110 event types - 89 async, 21 sync.

### attribute

| Event | Kind | What it means |
|---|---|---|
| `attribute.deleted` | async | Fires after an attribute is deleted, along with every value products held for it. Payload includes attributeId and slug. |
| `attribute.updated` | async | Fires after an attribute, its values, its product-type assignments or its flags change. Payload includes attributeId, slug and changedFields. |

### auth

| Event | Kind | What it means |
|---|---|---|
| `auth.login.attempting` | sync | Fires before credentials are verified, for both admin and customer logins. Reject to block the attempt (IP allow/deny lists, geo-anomaly rules, external rate limits). Payload includes subject (admin\|customer), email, ip, userAgent, and channelId (null for admin). Response schema: {"reject": bool, "reason": string}. |
| `auth.login.failed` | async | Fires after a failed login attempt. Subscribe for brute-force / credential-stuffing alerting. |
| `auth.login.succeeded` | async | Fires after a successful login. Payload includes the actor + IP; no credentials. |
| `auth.logout` | async | Fires after a user (admin or customer) logs out a single session. Payload includes subject, subjectId, and allDevices (false for a single-device logout). |
| `auth.password.reset.completed` | async | Fires after a password is successfully changed via the reset-token flow (distinct from customer.password_changed, the logged-in self-service change). Payload includes the subject, subjectId, email, and channelId. Subscribe to send a 'your password was changed' notice or revoke downstream sessions. |
| `auth.password.reset.requesting` | sync | Fires before a password-reset token is minted. Reject to refuse the reset (e.g. SSO-only accounts that must not reset via email). A rejection is applied silently - the caller sees the same response as an unknown email - to preserve the enumeration defense. Response schema: {"reject": bool, "reason": string}. |
| `auth.refresh.issued` | async | Fires when a refresh token is minted - on login and on every refresh rotation, for both admin and customer. Payload includes subject, subjectId, sessionId (admin only), ip, userAgent. Chatty; subscribe for session-count metrics or concurrent-session caps. |
| `auth.refresh.revoked` | async | Fires when a refresh token is revoked. Payload includes subject, subjectId, and reason (rotation \| logout \| logout_other_devices \| revoke_all). Subscribe to cascade session revocation to dependent systems. |

### backinstock

| Event | Kind | What it means |
|---|---|---|
| `backinstock.requested` | async | Fires when a shopper subscribes to a restock alert for a product. Payload: ChannelID, ProductID, SKU, Email, CustomerID (null for guests). |

### blog

| Event | Kind | What it means |
|---|---|---|
| `blog.post.content.changed` | async | Fires after a post's body/content changes. Can be chatty during editing - subscribe only if you mirror content. |
| `blog.post.created` | async | Fires after a blog post is created (draft or published). |
| `blog.post.deleted` | async | Fires after a blog post is deleted. |
| `blog.post.listing.changed` | async | Fires when a post's per-store listing changes. |
| `blog.post.published` | async | Fires after a blog post moves to status=published - admin click, scheduler promotion, or any other path. Useful for static-site rebuild triggers. |
| `blog.post.publishing` | sync | Fires before a blog post moves to status=published - admin click or scheduler promotion. A scheduled post that is rejected stays scheduled and is asked again on the next run. Response schema: {"reject": bool, "reason": string}. |
| `blog.post.unpublished` | async | Fires after a blog post moves out of status=published into draft or archived. |
| `blog.post.updated` | async | Fires after a blog post's metadata is updated. |

### bundle

| Event | Kind | What it means |
|---|---|---|
| `bundle.created` | async | Fires after a bundle product is created. Payload includes productId and SKU. |
| `bundle.deleted` | async | Fires after a bundle product is deleted. Payload includes productId and SKU. |
| `bundle.updated` | async | Fires after a bundle product is updated. Payload includes productId, SKU, and changedFields. |

### cart

| Event | Kind | What it means |
|---|---|---|
| `cart.bundle.added` | async | Fires after a bundle is added to a cart (the bundle counterpart to cart.line.added). Payload includes cartId, the bundle SKU, and the picked components. |
| `cart.bundle.adding` | sync | Fires before a bundle line is added. Reject to block the add. Response schema: {"reject": bool, "reason": string, "unitPriceCents": int, "quantity": number}. unitPriceCents overrides the bundle's summed component price. |
| `cart.cleared` | async | Fires after every line is removed from a cart. Payload includes cartId, channelId, customerId, anonId. Response ignored (async). |
| `cart.line.added` | async | Fires after a line is added to a cart. Payload includes cartId, channelId, customerId (null for guests), anonId, lineId, productSku, unitPrice, quantity. Response ignored (async). |
| `cart.line.adding` | sync | Fires before a simple line is added. Reject to block the add. Response schema: {"reject": bool, "reason": string, "unitPriceCents": int, "quantity": number}. unitPriceCents/quantity override the line being added (external dynamic pricing, per-customer clamps); omit to keep the input. |
| `cart.line.removed` | async | Fires after a line is removed from a cart. Payload includes cartId, lineId. Response ignored (async). |
| `cart.line.removing` | sync | Fires before a line is removed. Reject to block the removal. Response schema: {"reject": bool, "reason": string}. |
| `cart.line.updated` | async | Fires after a cart line's quantity changes. Payload includes cartId, lineId, quantity. Response ignored (async). |
| `cart.line.updating` | sync | Fires before a line quantity changes. Reject to block the change. Response schema: {"reject": bool, "reason": string, "quantity": number}. quantity overrides the absolute new line quantity. |
| `cart.merged` | async | Fires after a guest cart merges into a customer cart on login (guest→customer conversion). Payload includes customerCartId, guestCartId, customerId, channelId. Response ignored (async). |
| `cart.merging` | sync | Fires before a guest cart is merged into a customer's cart at login. Response schema: {"reject": bool, "reason": string}. |

### category

| Event | Kind | What it means |
|---|---|---|
| `category.created` | async | Fires after a catalog category is created. |
| `category.deleted` | async | Fires after a category is deleted. |
| `category.updated` | async | Fires after a category is updated (name, parent, URL, etc.). |

### channel

| Event | Kind | What it means |
|---|---|---|
| `channel.deleted` | async | Fires after a store/channel is deleted. |
| `channel.locales.changed` | async | Fires after a store's active locale set changes. |
| `channel.updated` | async | Fires after a store's own fields change - name, currency, locales, active flag, public URL or default flag. Subscribe to bust a headless frontend's nav cache. |

### checkout

| Event | Kind | What it means |
|---|---|---|
| `checkout.validating` | sync | Fires when checkout is fully resolved (cart, addresses, payment provider, engine discount) but BEFORE the placement transaction opens - the preferred veto point for fraud screens, address checks and approval gates since a slow endpoint here never pins a DB transaction. Response schema: {"reject": bool, "reason": string, "discountCents": int, "riskScore": int, "tags": [string]}. Enrichment is seeded onto the order.placing payload that follows; discounts accumulate, risk score takes the max, tags append. |

### cms

| Event | Kind | What it means |
|---|---|---|
| `cms.page.published` | async | Fires after a CMS page transitions to is_published=true. Subscribe to invalidate caches or kick off a static rebuild. |
| `cms.page.publishing` | sync | Fires before a CMS page transitions to is_published=true - admin click, bulk publish or scheduler promotion. A scheduled page that is rejected stays scheduled and is asked again on the next run. Response schema: {"reject": bool, "reason": string}. |
| `cms.page.unpublished` | async | Fires after a CMS page transitions back to is_published=false. |

### collection

| Event | Kind | What it means |
|---|---|---|
| `collection.created` | async | Fires after a collection is created. |
| `collection.deleted` | async | Fires after a collection is deleted. |
| `collection.products.changed` | async | Fires when the set of products in a collection changes (manual collections). Payload includes the collection id. |
| `collection.updated` | async | Fires after a collection is updated. |

### consent

| Event | Kind | What it means |
|---|---|---|
| `consent.changed` | async | Fires after a customer opts in or out of a consent kind - marketing_email, marketing_sms and so on. Payload: CustomerID, Kind, OptedIn, Source. Account-global, so per-channel webhooks never match it. |

### customer

| Event | Kind | What it means |
|---|---|---|
| `customer.address_changed` | async | Fires after an address book entry is added, updated or deleted, by the customer or an admin, or made the default. Payload includes the action: add, update, delete, default_billing or default_shipping. |
| `customer.deleted` | async | Fires after an admin deletion commits. Payload includes the customer's last-known email and channel_id so downstream systems can purge. |
| `customer.deleting` | sync | Fires before an admin customer delete commits. Use to refuse the delete based on external state (open tickets, active subscriptions). Response schema: {"reject": bool, "reason": string}. |
| `customer.email_verified` | async | Fires after a customer confirms their email via the verification link. Useful for moving the customer to a marketing-opted-in list or granting a welcome incentive. |
| `customer.password_changed` | async | Fires after a logged-in customer changes their own password (distinct from the reset-token flow). |
| `customer.profile_updated` | async | Fires after a customer edits their own profile, or a customer import changes one. Payload includes changedFields, named in camelCase like customer.updated's (firstName, lastName, phone), and Source: self or import. |
| `customer.registered` | async | Fires after a new customer registration commits. Response is ignored (async). |
| `customer.registering` | sync | Fires before customer registration commits. Blocks the flow until response. Response schema: {"reject": bool, "reason": string, "tags": [string]}. Returned tags are appended to the customer's registration tags. |
| `customer.updated` | async | Fires after an admin updates a customer, including the active toggle, group, tax status and VAT id. Response is ignored (async). Payload includes changedFields listing which fields were modified, in camelCase. |
| `customer.updating` | sync | Fires before an admin customer update commits. Blocks the flow until response. Response schema: {"reject": bool, "reason": string, "tags": [string]}. Returned tags are forwarded to the customer.updated async event. |

### download

| Event | Kind | What it means |
|---|---|---|
| `download.accessed` | async | Fires when a buyer redeems a download token (a file is served). |

### giftcard

| Event | Kind | What it means |
|---|---|---|
| `giftcard.adjusted` | async | Fires after an admin balance correction (top-up, goodwill, chargeback). Payload includes giftCardId, signed delta, new balance, and note. |
| `giftcard.issued` | async | Fires after a gift card is minted - an admin issue, or one card per gift-card line on a placed order. Payload includes giftCardId, code, currency, amount (opening balance, cents), customerId, and orderId (set when issued from a purchase). |
| `giftcard.redeemed` | async | Fires after a gift card is spent via the standalone redeem path. Payload includes giftCardId, code, amount applied, and remaining balance. (Checkout-tender redemption applied inside the order transaction is not emitted here.) |

### inventory

| Event | Kind | What it means |
|---|---|---|
| `inventory.allocating` | sync | Fires before stock is reserved for an order line. Reject to refuse the allocation. Response schema: {"reject": bool, "reason": string, "routing": {"<orderLineId>": "<locationId>"}}. Routed lines allocate from exactly the picked location (warehouse routing, store pickup); unrouted lines use the default algorithm. |
| `inventory.stock.changed` | async | Fires on any salable-quantity change for a product at a location. Chatty - payload includes productId, locationId, and the new quantity. |
| `inventory.stock_low` | async | Fires when a stock level transitions across the per-item low_stock_threshold (from above to at-or-below the threshold while still positive). |
| `inventory.stock_out` | async | Fires when a stock level reaches zero from a positive quantity. |

### menu

| Event | Kind | What it means |
|---|---|---|
| `menu.updated` | async | Fires after a storefront menu (navigation) is updated. Subscribe to bust a headless frontend's nav cache. |

### order

| Event | Kind | What it means |
|---|---|---|
| `order.cancelled` | async | Fires after an order transitions to cancelled. More specific alternative to filtering order.status_changed on toStatus. |
| `order.cancelling` | sync | Fires before a cancel transition commits. Blocks the flow until response. Response schema: {"reject": bool, "reason": string, "note": string}. Use to gate on refund success or to append a note onto the order_event audit row. |
| `order.edited` | async | Fires after an admin edits a placed order (lines/quantities/prices). Subscribe to re-sync external systems that mirror the order. |
| `order.fulfilled` | async | Fires after an order transitions to fulfilled. Subscribe here for shipping notifications, accounting closeout, or post-fulfillment surveys. |
| `order.fulfilling` | sync | Fires before a fulfilled transition commits. Blocks the flow until response. Response schema: {"reject": bool, "reason": string, "carrier": string, "trackingNumber": string}. Carrier + trackingNumber, if returned, are appended to the order_event message. |
| `order.fulfillment_created` | async | Fires when a (possibly partial) fulfillment/shipment is created for an order. Payload includes the fulfilled lines + tracking when set. |
| `order.payment_status_changed` | async | Fires when an order's payment state transitions (e.g. pending → paid → refunded). Payload includes orderId, fromStatus, toStatus. |
| `order.placed` | async | Fires after a new order commits. Response is ignored (async). |
| `order.placing` | sync | Fires before order placement commits. Blocks the flow until response. Response schema: {"reject": bool, "reason": string, "discountCents": int, "riskScore": int, "tags": [string]}. Discounts accumulate across webhooks; risk score takes the max; tags append. Tax/shipping/total are owned by the remote tax + shipping providers, not this webhook. |
| `order.return_approved` | async | Fires after a requested return is approved by a moderator/admin. |
| `order.return_received` | async | Fires when returned goods are marked received (the step after return_approved). Completes the RMA lifecycle. |
| `order.return_rejected` | async | Fires after a requested/approved return is rejected (terminal). |
| `order.return_requested` | async | Fires after a customer/admin opens a return (RMA) against a shipped order. Payload includes orderId and returnId. |
| `order.status_changed` | async | Fires after a non-placement order status transition commits. Payload includes orderId, fromStatus, and toStatus. Response is ignored (async). |
| `order.tendering` | sync | Fires inside the placement transaction once the order exists and core's gift-card and store-credit tenders have been applied, carrying the balance still owed - the veto point for a credit line or pay-later arrangement, which refuses by rejecting. Payload: OrderID, OrderNumber, ChannelID, CustomerID (null for guests), Email, Provider, CurrencyCode, TotalCents, RemainingCents. Response schema: {"reject": bool, "reason": string}. Published for every method with a balance, so match on Provider before acting. A slow endpoint here holds the placement transaction open - prefer checkout.validating for anything that only needs to say no. |

### payment

| Event | Kind | What it means |
|---|---|---|
| `payment.authorized` | async | Fires after a payment authorization succeeds (funds held, not yet captured). Payload includes orderId, provider, amount, currency. |
| `payment.captured` | async | Fires after funds are captured (money moves). The usual trigger for accounting + fulfillment. |
| `payment.provider_webhook` | async | Fires when an inbound payment-provider callback (e.g. Stripe/Braintree/PayPal) is received and processed - useful for reconciliation of async/delayed settlement events. |
| `payment.refunded` | async | Fires after a full or partial refund is issued. Payload includes the refunded amount. |
| `payment.voided` | async | Fires after an uncaptured authorization is voided (the hold is released). |

### product

| Event | Kind | What it means |
|---|---|---|
| `product.created` | async | Fires after a new product row commits. Typical subscribers: search index sync, CDN cache warmer, ERP catalog mirror. |
| `product.creating` | sync | Fires before a new product row inserts. Blocks the flow until response. Response schema: {"reject": bool, "reason": string, "tags": [string]}. Returned tags are joined and stored under metadata.webhook_tags. |
| `product.deleted` | async | Fires after a product row delete commits. Use the snapshot fields in the payload - the row is gone by the time this fires. |
| `product.listing.changed` | async | Fires when a product's per-store listing changes (published/unpublished or price override). Payload includes productId + channelId. |
| `product.updated` | async | Fires after a product row update commits. Payload includes changedFields listing the top-level fields that were modified. |
| `product.updating` | sync | Fires before a product update commits. Payload includes changedFields. Response schema: {"reject": bool, "reason": string, "tags": [string]}. Returned tags are merged into metadata.webhook_tags. |

### qna

| Event | Kind | What it means |
|---|---|---|
| `qna.question_answered` | async | Fires after an operator answers a question (which publishes it to the storefront). Payload includes questionId, productId, channelId, answeredBy, and both the question and answer text. |
| `qna.question_submitted` | async | Fires after a shopper submits a product question (pending moderation). Payload includes questionId, productId, channelId, customerId, authorName, and the question text. |

### ratelimit

| Event | Kind | What it means |
|---|---|---|
| `ratelimit.throttle_tripped` | async | Fires when an IP exhausts a request budget on the front API. Payload: IP, Surface, Count, Budget, ChannelID. Feed it to a WAF or alerting. |

### review

| Event | Kind | What it means |
|---|---|---|
| `review.approved` | async | Fires after a review is approved and becomes publicly visible. |
| `review.rejected` | async | Fires after a review is rejected in moderation. |
| `review.submitted` | async | Fires after a customer submits a product review (pre-moderation). Payload includes productId, rating, and the author. |
| `review.submitting` | sync | Fires before a product review is stored. Reject to refuse the submission. Response schema: {"reject": bool, "reason": string, "status": "approved"\|"rejected"\|"pending", "tags": [string]}. status short-circuits manual moderation (external spam services); tags append and survive onto review.submitted. String literal to avoid a webhook→review→ai import cycle; mirrors review/events.go. |

### subscription

| Event | Kind | What it means |
|---|---|---|
| `subscription.cancelled` | async | Fires after a subscription is cancelled (customer or admin). |
| `subscription.created` | async | Fires after a subscription is started from a placed order. |
| `subscription.invoiced` | async | Fires after a billing-period invoice is minted (one per period). Payload includes the amount + currency. |
| `subscription.paused` | async | Fires after an admin pauses a subscription (billing suspended until resumed). |
| `subscription.payment_failed` | async | Fires after a renewal charge fails. Attempt counts from 1; NextRetryAt is the next dunning step, null when the ladder is exhausted and WillCancel is true (the subscription was cancelled). |
| `subscription.resumed` | async | Fires after a paused subscription is reactivated. |

### urlrewrite

| Event | Kind | What it means |
|---|---|---|
| `urlrewrite.changed` | async | Fires after a url_rewrite row is created, updated, or deleted (a canonical URL moved or a redirect changed). Payload includes path, entityType, entityId, channelId (null = global), and deleted. Subscribe to purge the affected path from edge caches. |

### webform

| Event | Kind | What it means |
|---|---|---|
| `webform.submitted` | async | Fires after a storefront web form (contact, lead, custom) is submitted. Payload includes the form id + submitted fields. |

### wishlist

| Event | Kind | What it means |
|---|---|---|
| `wishlist.item_added` | async | Fires after a signed-in customer saves a product to their wishlist. Payload includes customerId and productId. |
| `wishlist.item_removed` | async | Fires after a customer removes a product from their wishlist. Payload includes customerId and productId. |
<!-- END GENERATED WEBHOOK EVENTS -->
