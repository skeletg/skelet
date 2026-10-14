# Cart offers

An offer is a cart an admin builds for one recipient and sends as a link. The recipient
completes it through the normal checkout. Use one for:

- a custom quote
- an order taken by phone or email
- a receipt for an extra payment

An offer is never the customer's own cart. It sits beside it, and it is never merged into
it.

## Building and sending one

**Orders → Offers → New offer**. The separate **Carts** page lists shopper carts;
**Offers** lists offers, with their recipient, state and expiry. Each page creates its own
kind of cart, and the detail page returns to that list. Make it out to:

- **An existing customer.** They must sign in to open it.
- **A new customer.** The account is created with it.
- **A guest email.** Anyone with the link can open it.

On the cart page:

- **Line items:** add catalog products as on any cart. Each line on an offer also has a
  price field.
  - A price you type is the offer price, and checkout keeps it instead of repricing from
    the catalog.
  - **Reset** puts a catalog line back on its catalog price.
- **Custom line:** a free-form charge with no product behind it, such as an extra payment,
  a service or a fee.
  - You give it a name, price, quantity and tax class.
  - It never ships, and it reserves no stock.
  - On the order it is marked `isCustom`.
- **Offer:** the expiry date and a message to the recipient. A customer offer can also be
  listed in their account under **Offers**, not only behind the link. The account
  navigation and overview show the active-offer count, with a dot when there are unopened
  offers. Those rows show **New** until the customer opens them.
- **Locked for the recipient:** the parts the recipient sees but can't change. See the
  locks table below.
- **Checkout:** the shipping and billing address, allowed payment methods and delivery the
  recipient starts from. Delivery choices are quoted by the storefront's own checkout for
  the saved shipping address, so save the address first. Anything left empty, the
  recipient fills in, unless it's locked.

Line edits apply straight away. The offer settings, locks and checkout are staged behind the
page-header **Save**.

The header actions are:

- **Preview email:** shows the recipient, subject, HTML and plain text without sending.
  It also lets you copy the link. Save staged changes first to preview the current terms.
- **Send:** emails the link, and sends again on every use.
- **Copy link:** to share it some other way.
- **Replace link:** every link handed out so far stops working.
- **Withdraw offer:** the offer can't be completed any more. This can't be undone.

**Send** waits until nothing is staged, so the email never describes old terms.

### Locks

| Lock | What the recipient can't change | Needs |
|---|---|---|
| Items and quantities | Adding, removing or changing lines | - |
| Billing address | The billing address | A billing address filled in |
| Shipping address | The shipping address and destination | A shipping address. Only on a cart that ships |
| Delivery | The delivery option, method and pickup point per group | A delivery chosen. Only on a cart that ships |
| Coupons, gift cards and store credit | Anything taken off the price | - |

Locks are enforced by the backend, whatever the storefront shows. A checkout step that
would change a locked part is refused whole with `CART_OFFER_LOCKED`. The recipient's email
on a guest offer is always fixed.

### Payment methods

**Checkout → Payment methods** lists the channel’s enabled methods. Leave the list empty
to allow any eligible method, select one to pre-select that method, or select several to
let the recipient choose among them. Checkout still applies the normal country eligibility
rules. The backend refuses a method outside the list with
`CART_OFFER_PAYMENT_NOT_ALLOWED`, both when setting it and when placing the order.

## The link

The link is `<channel public URL>/offer/<token>`. The channel needs a public URL before an
offer can be sent.

The token is `<cart id>.<link version>.<mac>`. It is an HMAC signature, derived from
`JWT_SECRET` and kept separate from its other uses, so nothing is stored:

- **Showing it again:** the console can always show the link.
- **Replacing it:** bumps the cart's link version, which kills every older token.
- **Expiry:** read from the cart each time the link is used, so extending an offer needs no
  new link.

The storefront route `app/offer/[token]` handles a followed link:

1. It calls `openCartOffer`.
2. If the offer was made out to an account and nobody is signed in, it sends the shopper to
   sign in, then back.
3. It keeps the token in an httpOnly cookie and sends it as `X-Cart-Offer` on every call.

Without the header, an offer's id opens nothing. An offer listed in a customer's account
opens for that customer without the link.

Checkout works on the offer while the `storefront_checkout_offer` cookie is set:

- The offer page's **Continue to checkout** sets it.
- The shopper's own cart links clear it.
- Placing the order clears both offer cookies and leaves the shopper's own cart untouched.

A guest offer places a guest order for its recipient, even when someone is signed in on
that browser.

## Lifetime

| State | When |
|---|---|
| Draft | Not sent yet |
| Sent | Emailed at least once |
| Opened | The recipient followed the link |
| Completed | Placed - the console links to the order |
| Expired | Past its expiry - the link explains, and checkout refuses |
| Withdrawn | Revoked by an admin |

New offers expire after `sales/cart/offer_expiry_days` (default 7, per channel), and each
offer's date can be changed.

Offers are not:

- **Pruned:** the cart retention job skips an open offer, however long it sits.
- **Treated as abandoned carts:** abandoned-cart recovery skips offers.

An offer can be placed even where `sales/checkout/allow_guest` is off or the order is
below `sales/checkout/min_order`. The admin chose its recipient and amount.

## Email and events

**Send** offers the mail to a Pulse flow on the `cart_offer` trigger first. A seeded
**Cart offer** flow and email exist for it. When no flow takes the mail, core sends its own
template (`lib/email/cart_offer_template.go`).

**Preview email** renders core’s template using the same composition as its send path.
An active Pulse flow may send its own design instead. If the channel has no public URL,
the preview shows the relative link path and marks the link incomplete; sending needs
the public URL configured under **Settings → Identity**.

The flow email can use these merge tags:

- `{{offer.link}}`
- `{{offer.expiresAt}}`
- `{{offer.message}}`

Webhooks `cart.offer.sent`, `cart.offer.opened` and `cart.offer.expired` report each step
([webhooks.md](webhooks.md)). The `cart-offer-expiry` job announces expiries every five
minutes.

## Where it lives

| Part | Path |
|---|---|
| Model, locks, link, access | `internal/cart/offer*.go` - an offer is a `carts` row with `kind = 'offer'` |
| Admin API | `graph/schema/admin/carts.graphqls`, `graph/admin/cart_offer_helpers.go` |
| Storefront API | `openCartOffer`, `myCartOffers` and `Cart.offer` in `graph/schema/front/cart.graphqls` |
| Console | `apps/dashboard/src/features/carts/offer/` |
| Storefront | `app/offer/[token]/route.ts`, `app/[channel]/[locale]/cart/offer/`, `app/[channel]/[locale]/account/offers/` |
