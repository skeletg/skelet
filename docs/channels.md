# Channels, scope and locale

How one Skelet install serves several storefronts, and what each of them can vary.

New here? [concepts.md](concepts.md) defines the vocabulary this page uses -
channel, scope, listing, the scope ladder - in a page you can read in one sitting.
This one is the full behaviour.

A **channel** is one subsidiary storefront: a market,
a brand, a language edition. It carries the commerce policy for that market - its currency,
which locales it serves, its public URL, whether prices are quoted tax-inclusive. The
catalog underneath is shared - one product row, however many storefronts - and the channel
decides which products that storefront shows and at what price. Customers and orders are
not shared; each belongs to the channel it was created on.

Two facts do most of the work in understanding the rest of this page:

- **A channel has exactly one currency.** Selling in two currencies means two channels.
- **Translations are not keyed by channel.** Content is keyed by *locale only*, an
  orthogonal axis. Two channels serving `de` show the same German text for a product,
  because there is one German translation, not one per channel.

## Picking a channel

**One storefront app serves exactly one channel.** It is pinned per deployment by
`STOREFRONT_CHANNEL_ID` and sends that uuid as `X-Channel-Id` on every request. Serving a
second market means a second channel *and* a second deployment - never one app switching
between them. A production storefront with the variable unset refuses to serve rather than
guess, because guessing means quietly rendering someone else's market.

**On the front API**, the backend resolves one channel per request, in this order:

1. The `X-Channel-Id` header, a channel UUID. This is what a storefront app always sends.
2. Otherwise the request's host, matched against the channel's configured storefront host.
3. Otherwise the channel flagged as default - at most one may be, and it's opt-in.

Steps 2 and 3 exist for the other callers of the front API - a mobile client, a curl, an
integration - not for storefronts, which are pinned and never rely on them.

Every step requires the channel to be **active**; an inactive channel is skipped as though
it did not exist, including as the host match. If none resolve, the request simply has no
channel, and resolvers that need one fail with `STORE_REQUIRED`.

**A header that is present but doesn't resolve stops there** - unknown id, inactive channel
or malformed uuid all end resolution with no channel rather than falling through to host and
default. An explicit id is a directive: honouring a stale one by serving a different market's
catalog, prices and currency would look like it worked, which is worse than an error. That
also means deactivating a channel takes its storefront dark instead of quietly redirecting
its traffic somewhere else.

**In the admin**, the same header names your *viewing scope*, falling back to
`X-Store-Slug`. The rule is deliberately different: **no header means all channels, not the
default one.** Treat them as one rule and a global-scope admin silently sees only the
default store.

In the dashboard this is the scope selector in the header. It has two modes - **Global** and
one specific **Channel** - plus an optional locale, and it persists across page loads. It
decides what you are editing, so a setting saved in Global scope becomes the default for
every channel, while the same setting saved in Channel scope overrides it for that one.
Some screens require a channel and disable Global; if your grants are channel-scoped, the
header drops Global entirely and snaps you into a channel you can actually use.

## What varies per channel

Data falls into four patterns. Knowing which one an entity uses tells you what a second
channel actually costs you.

**1. Global - one row, identical everywhere.** Products, categories, collections,
attributes, product types, CMS pages, blog posts, menus, inventory items and locations,
locales, currencies. Editing one of these changes every channel at once.

**2. Global, with a per-channel listing.** The entity is global; a junction row decides
whether it appears on a channel and how. This is the pattern that makes multi-channel
retail work, and product listings are the fullest example - a listing carries
`is_published`, `price`, `compare_at_price`, `special_price` with its own from/to window,
and `url`. So the same product can be live in the US at one price, unpublished in the EU,
and on sale in the UK, with one catalog row underneath. Categories, CMS pages, blog posts,
menus and inventory locations use the same shape.

**3. Belongs to exactly one channel.** Carts, orders, **customers**, reviews, companies and
B2B quotes, tax rules, shipping zones, carriers and boxes, payment methods, experiments,
flows, email campaigns and templates library, signup and web forms, search boosts, pins and
redirects.

**4. Global by default, overridable per channel.** These carry a nullable channel id where
null means "applies everywhere": promotion rules, URL rewrites, webhooks, config values,
email templates, search synonyms, customer-group prices, per-product attribute slugs,
remote providers, subscriptions.

Three of these surprise people:

- **Customers are per-channel.** The same person signing up on `us` and on `eu` is two
  customer records with two passwords and two order histories. A session issued for one
  channel is refused on another. If you want one account across markets, that is one channel
  with several locales, not several channels.
- **Inventory is global but availability is per-channel.** There is one stock figure per
  item; a junction of (channel, location, priority) decides which locations a channel sells
  from and in what order they're drawn down. Two channels can serve different warehouses
  without duplicating stock, and the same warehouse can back both.
- **Content is per-locale, never per-channel** - covered above, and the single most common
  source of "why did editing the EU store change the US one?"

## Settings and the scope ladder

Settings are the clearest case of scope in action. Every setting resolves over three levels,
most specific winning:

```
default  (no channel, no locale)   ← the base value
  └─ channel                       ← this channel overrides the base
       └─ channel + locale         ← this channel, in this language
```

So a support email can be set once globally, overridden for the EU channel, and overridden
again for EU's French pages, with each level falling back to the one above where nothing is
set.

Not every setting can be overridden. Each one declares its scoping in code as either
**global** (settable only at the base level - things that must be true install-wide) or
**channel** (overridable). A setting that doesn't declare one inherits from its group, then
its section, then defaults to global. In practice: if the dashboard won't let you change a
setting while a channel is selected, it's a global-scoped setting, and you need to switch
the header to Global scope to edit it - which changes it everywhere.

## Locale

The request's content locale comes from the `Locale` header - a single BCP-47 tag, not a
weighted `Accept-Language` list. A malformed tag is ignored rather than carried, so junk
can't reach cache keys and search term filters.

**On the storefront the locale is clamped to the channel's supported set.** If the header
asks for a locale this channel doesn't serve, it snaps to the channel's default rather than
returning empty content. The clamp is case-insensitive but returns the canonical stored
code, so cache keys and index filters stay uniform. One deliberate exception: a locale
deactivated globally is skipped *unless* it is the channel's default, so deactivating a
locale can never take a storefront dark.

**In the admin the locale is not clamped**, on purpose - translation tooling has to be able
to edit a locale the channel doesn't currently serve.

With no usable header the resolution order is: the active channel's default locale, then the
*default* channel's default locale, then `en`.

Missing translations fall back rather than blanking: an untranslated field keeps the
canonical value.

## Currency

Each channel holds one currency, and that is what the cart charges and the order settles in.

The storefront can additionally *display* prices in other currencies through the
`presentmentCurrencies` query, which returns a conversion factor per currency computed from
stored rates. Two things to know: a currency whose rate hasn't been refreshed in seven days
is omitted rather than shown stale, and this is **presentment only** - checkout still
charges the channel's own currency. Selling and settling in a second currency means a second
channel.

`tax_inclusive_pricing` is also per channel. With it on, prices are stored gross and the tax
service splits the rate out of the gross amount at checkout instead of adding on top - the
normal arrangement for EU consumer stores.

## Common setups

**One market per channel** - `us`, `eu`, `uk`. Different currency, different tax treatment,
usually a different host. This is the case channels are designed for.

**One brand per channel** on a shared catalog, differing by theme, URL and which products
are listed.

**A language edition** is usually *not* a separate channel. One channel serving several
locales is the normal way; add a locale to the channel rather than cloning it. Reach for a
second channel when the *commerce policy* differs - currency, tax, pricing - not when only
the words do.

The exception worth knowing: because content is keyed by locale and not by channel, two
channels sharing a locale share its text. If you need genuinely different copy for the same
language in two markets, give them distinct locale codes (`en-US` and `en-GB`) rather than
expecting the channel to separate them. This is also why an admin scoped to one channel is
refused edits to shared content - see [permissions](permissions.md).

---

**Implementing against this?** [commerce-model.md](commerce-model.md) carries the
engineering side: the dashboard scope hook, the Go listing types, and the
per-domain footguns.
