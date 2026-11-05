# Privacy: consent, tracking and what is recorded

What the shop records about visitors, under which cookie choice, and what to say in your privacy notice. This
describes how Skelet behaves. It is not legal advice: check the rules where you sell with someone qualified.

## The cookie banner

Visitors choose three categories. They follow Google's Consent Mode v2.

| Category | Covers |
|---|---|
| Analytics | The `lens_aid` visitor id, Signals profiles, analytics destinations such as GA4 |
| Advertising | Ad click ids (`gclid`, `msclkid`, `_fbp`, `_fbc` and the rest), ad destinations such as Meta and Bing |
| Personalization | Personalized marketing destinations |

`general/privacy/cookie_consent` picks the model per shop:

- **Ask first (opt-in)** is the EU and UK model. Nothing in a category is set until the visitor allows it.
- **No banner (opt-out)** is the US model. Everything is on, a "Your privacy choices" footer link turns it off, and a
  browser's Global Privacy Control signal turns advertising off.

The choice is stored in the `storefront_consent` cookie. It is strictly necessary, so it needs no consent itself.

**Affiliate cookies.** CJ's `cje` and Awin's `awc` need advertising consent by default.
`general/privacy/cj_cookie_necessary` and `general/privacy/awin_cookie_necessary` let them work without it, which some
programmes require. Whether that is lawful where you sell is your decision.

## Orders follow the consent given at purchase

Each order keeps a snapshot of the visitor's choices. Each part is kept only under the category it needs:

| Part | Needs |
|---|---|
| Visitor id | Analytics |
| Click ids, IP address, browser | Advertising, or a necessary affiliate network |

Events sent later about the order, such as a refund weeks after the purchase, follow the choices made at purchase,
not the visitor's current ones. GDPR erasure clears the snapshot with the rest of the order's personal data.

## Server-side destinations

Purchases, refunds, cart changes and sign-ups go from the backend straight to the platforms you configure. Product
views and checkout steps go through the shop's own `/t/e` path. No third-party script runs in the shopper's browser.

An event reaches a destination only when the visitor allowed every category that destination needs. An order placed
by an admin or an API client has no banner choice behind it, so it reaches only destinations that need no consent.

Each event is stored once while it waits to be sent. Within an hour of its last delivery, its personal data (email,
phone, IP, click ids) is deleted. Only its key stays, for 48 hours, so a replay can't send it twice. A delivery that
failed keeps its event for 30 days so it can be retried. Erasing a customer deletes their waiting and failed events.

**Leaving a list needs no consent.** When a customer turns email marketing off, or their account is deleted or
erased, destinations built to remove people - Google Ads Customer Match - are told whatever the visitor's cookie
choice. Every other destination still needs the visitor's consent for it.

**Customer Match** puts account holders who opted into email marketing, and allowed advertising and personalization
cookies, on a Google Ads customer list as hashed email and phone. If you use it, say so in your privacy notice, for
example: "If you subscribe to our marketing emails, we may share a hashed copy of your email address and phone number
with Google so that it can show you our ads. You can stop this at any time by unsubscribing."

## Counting visitors who decline analytics

With `general/privacy/aggregate_counts` on (the default), Signals still counts activity from visitors who haven't
allowed analytics:

- page views
- cart changes
- orders and revenue

They are counted as nobody, so that the shop's totals stay right:

- **Nothing is stored on or read from their device** for this. The page view is counted by the shop's server from
  the request it serves anyway.
- **No identifier is recorded**: no account, visitor id, email, IP address or browser.
  - Page paths are kept without their query string, and with ids in them (an order number, a reset token) replaced
    by `:id`.
  - Only amounts and items are kept from cart and order activity.
  - Times are kept to the hour, and the key that stops an event being counted twice is hashed with a secret neither
    database holds, so neither can be matched back to an order.
- **They never appear as a person.** They appear in event totals and revenue. They are left out of unique-visitor
  counts, funnels, retention, profiles, RFM and segments.
  - The Insights page says how much of a total they make up.

The legal basis for this is legitimate interest: knowing how the shop is used overall. Before relying on it, write
down a short legitimate-interest assessment, and mention it in your privacy notice. For example:

> When you decline analytics cookies, we still count page views, basket changes and orders in our overall shop
> statistics. We do this without cookies and without recording anything that identifies you, your device or your
> browser, and these figures are only ever used as totals. Our legal basis is our legitimate interest in
> understanding how our shop is used.

Turn the setting off and nothing is recorded for visitors who decline analytics.

## Marketing email

Accepting cookies never subscribes anyone to marketing email. That is a separate, unticked checkbox:

- **At sign-up**, it sets the account's email consent.
- **At checkout**, a signed-in shopper's tick goes to their account. A guest's tick is kept under the order's email,
  with the order number as proof.
  - It moves onto an account once someone signs up and verifies that address, unless the account made a later choice.
  - Data exports list it, and erasure deletes it.
