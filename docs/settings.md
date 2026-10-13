# Settings

Every configurable knob in the store: where it lives, what scope it can be set at, and what
happens when it isn't set.

Settings are edited on the **Settings** page in the admin. They are also the mechanism
behind a lot of things that don't look like settings - search tuning, checkout behaviour,
retention windows, feature switches - so this page doubles as a map of what is tunable
without a deploy.

## How a path is built

Every setting has a three-part path:

```
section / group / field
     │       │       └── the input itself
     │       └────────── the fieldset it renders in
     └────────────────── the tab in the left nav
```

So `outreach/forms/enabled` is the *Enabled* field, in the *Signup Forms* group, on the *Outreach* tab.
The path is the identity of the setting - it is what appears in the reference below, what
the API takes, and what stays stable if a label is reworded.

## Scope, and how a value resolves

A setting resolves over three levels, most specific winning:

```
default  (no channel, no locale)   ← the base value
  └─ channel                       ← overrides the base for one channel
       └─ channel + locale         ← overrides it for one language on that channel
```

Nothing set at a level means it falls through to the level above, and past the base level it
falls back to the **default** shipped in code - which is the *Default* column in the
reference.

Not every setting is overridable. The **Scope** column says which:

- `global` - settable only at the base level. Things that must be true install-wide. If the
  admin won't let you change a setting while a channel is selected, this is why: switch the
  header to Global scope, and understand you are changing it everywhere.
- `channel` - overridable per channel, and per locale within a channel.

A setting that doesn't declare its own scoping inherits from its group, then its section,
then defaults to `global`. The reference resolves that inheritance for you, so the column is
the effective answer rather than the raw declaration.

**Locale is not a scope of its own.** There are only two scope kinds, `default` and
`channel`, and a locale is an optional refinement *inside* channel scope. A value at the
base level may not carry one at all - that's rejected, not ignored - so a `global` setting
can never vary by language. And there is no way to say "this value, in German, everywhere":
you set it per channel, on each channel that serves German.

Worth pinning down because [permissions](permissions.md) use the word differently - there,
locale really is an independent axis, and a grant of `(any channel, de)` is valid and means
German content on every channel. Same vocabulary, different shape.

For how channel scope works more broadly, see [channels](channels.md).

## Types

Most types are self-explanatory (`string`, `int`, `bool`, `select`). Four are worth knowing:

- **`secret`** - encrypted at rest and masked by the admin API, so the form shows an empty
  field even when a value is set and saving it empty leaves the stored one alone rather than
  clearing it. Gateway keys and the like. Defaults show as `-` here because printing one
  would publish a credential. Encryption needs a configured key; without one (dev) the value
  is stored as plaintext, which is why production boot refuses to start without it.
- **`json`** - any structured value, stored canonicalised, so what you read back is
  whitespace-normalised rather than byte-identical to what you typed.
- **`image`** - either a plain URL or a reference into the media library; opaque to the
  backend.
- **`multiselect`** and **`select`** - options can come from a static list or be generated
  at read time from the database (customer groups, for instance), so the choices you see
  depend on your data.

## Public settings

Settings marked **Public** in the reference are served by an *unauthenticated* query,
because the storefront needs them before anyone signs in - branding, currency display, which
features are switched on. That is a meaningful slice of the registry, so treat the Public
column as the list of values visible to the entire internet.

Never put a credential in one. `secret` and `public` are independent flags, and the public
query filters on `public` alone - it does not exclude secret-typed settings, and the masking
that hides them happens in the admin layer rather than underneath. A setting marked both
would therefore serve its decrypted value to anonymous callers.

That combination is now **rejected at startup**: the registry check refuses to validate a
definition that is both, so the server fails to boot with the offending path named rather
than quietly exposing it.

## Reference

Generated from the code-defined registry by `bin/codegen`, so it lists what the running
server actually honours. Within a group, the order matches the admin's rendering order.

**What it does** is the setting's registry description, which is also rendered as help text
under the field in the admin. Descriptions are being backfilled, so where none is written
yet the column falls back to the label - useful for finding the knob, less so for
understanding it. If a row reads like a restatement of its own path, that's a description
waiting to be written rather than a setting with nothing to say.

<!-- BEGIN GENERATED SETTINGS -->
<!-- Generated by cmd/docsref via bin/codegen. Edits here are overwritten. -->

154 settings in 14 sections.

### General

**Store Information**

| Path | What it does | Type | Scope | Public | Default |
|---|---|---|---|---|---|
| `general/store_information/support_phone` | Served to storefronts over publicConfig; the bundled one doesn't show it. | phone | channel | yes | - |
| `general/store_information/description` | A short line about the store. Served to storefronts over publicConfig; the bundled one doesn't show it. | textarea | channel | yes | - |
| `general/store_information/phone_mobile` | A second number, for stores that publish one. Served to storefronts; the bundled one doesn't show it. | phone | channel | yes | - |
| `general/store_information/currency_list` | The currency every stored exchange rate is quoted against, refreshed twice daily from ECB. Storefront prices come from each channel's own currency, not from here. | select | global | yes | `USD` |
| `general/store_information/working_hours` | Per-day opening times. Served to storefronts over publicConfig; the bundled one doesn't show them. | working_hours | channel | yes | `{}` |
| `general/store_information/timezone` | Sets the timezone of the clock shown in the admin sidebar. Report boundaries, scheduled jobs and stored timestamps are unaffected - those are UTC, or carry their own timezone. | select | channel |  | `UTC` |

**Security**

| Path | What it does | Type | Scope | Public | Default |
|---|---|---|---|---|---|
| `general/security/admin_single_session` | Signing in revokes every other admin session for that account. Stops a shared login being used from two places, at the cost of forcing a re-login when someone moves between machines. | bool | global |  | `false` |
| `general/security/catalog_query_throttle` | Soft per-IP budget for storefront catalog queries. Past the budget requests are slowed rather than refused; past three times it they are refused outright. 0 disables both. | int | channel |  | `0` |
| `general/security/require_email_verification` | Require verified email to sign in | bool | channel |  | `false` |

**URLs & Redirects**

| Path | What it does | Type | Scope | Public | Default |
|---|---|---|---|---|---|
| `general/urls/auto_redirect_on_change` | Keeps old links working after a slug edit. Off means the previous URL 404s. | bool | global |  | `true` |
| `general/urls/redirect_type` | Automatic redirect type | select | global |  | `301` |
| `general/urls/url_template_fallback` | Off means the save is rejected instead, which catches a broken template rather than papering over it. | bool | global |  | `true` |
| `general/urls/redirect_retention_days` | Only auto-created redirects are pruned - ones you added by hand are kept. | int | global |  | `365` |
| `general/urls/variant_own_url` | Each variant becomes its own indexable page. More pages to crawl, but a shopper landing from search arrives on the exact one. | bool | channel | yes | `true` |

**Country Options**

| Path | What it does | Type | Scope | Public | Default |
|---|---|---|---|---|---|
| `general/country/allowed` | Allowed countries | multiselect | channel | yes | - |
| `general/country/default` | Default country | select | channel | yes | - |
| `general/country/required_region` | Countries requiring a region/state | multiselect | channel | yes | - |
| `general/country/allow_optional_state` | Show optional state field | bool | channel | yes | `false` |
| `general/country/top_destinations` | Top destinations | multiselect | channel | yes | - |
| `general/country/eu_countries` | EU countries (VAT grouping) | multiselect | channel | yes | - |
| `general/country/phone_countries` | Any country takes every valid number, typed with + and its country code. Allowed countries only takes numbers from the countries above - every active country when none are picked - and a store with one country takes only that country's numbers. | select | channel | yes | `any` |

**Branding**

| Path | What it does | Type | Scope | Public | Default |
|---|---|---|---|---|---|
| `general/branding/store_name` | The name transactional emails sign off with. Empty falls back to the channel's own name, which is the operator-facing label. Also served to storefronts, though the bundled one shows the channel name. | string | channel | yes | - |
| `general/branding/logo_url` | Shown at the top of every outbound email, and used in place of the store name wherever a storefront has a brand slot. | image | channel | yes | - |
| `general/branding/favicon_url` | The tab icon. A square PNG or ICO of at least 32x32; storefronts fall back to their own bundled icon when unset. | image | channel | yes | - |
| `general/branding/primary_color` | The dominant brand color - storefronts use it for buttons and other solid fills. Empty keeps their own palette. | color | channel | yes | - |
| `general/branding/secondary_color` | The accent beside it, for highlights and small emphasis rather than large areas. | color | channel | yes | - |

**Contact**

| Path | What it does | Type | Scope | Public | Default |
|---|---|---|---|---|---|
| `general/contact/support_email` | The address to show customers when they need help - not the one mail is sent from. Served to storefronts; the bundled one doesn't show it. | string | channel | yes | - |
| `general/contact/email_from_address` | The From address on every email the store sends. It has to be one your mail provider will accept, or delivery fails. | string | channel |  | - |
| `general/contact/email_from_name` | The From display name shown beside the address. | string | channel |  | - |

**Legal**

| Path | What it does | Type | Scope | Public | Default |
|---|---|---|---|---|---|
| `general/legal/terms_url` | Served to storefronts to link from checkout and the footer; the bundled one doesn't. | string | channel | yes | - |
| `general/legal/privacy_url` | As above - served, not linked by the bundled storefront. | string | channel | yes | - |

### Sales

**Checkout**

| Path | What it does | Type | Scope | Public | Default |
|---|---|---|---|---|---|
| `sales/checkout/allow_guest` | Whether someone can complete checkout without an account. Off forces registration or sign-in before the order can be placed. | bool | channel | yes | `true` |
| `sales/checkout/min_order` | Orders below this merchandise total are refused at checkout. In minor units, so 2500 is $25.00. 0 disables the floor. | int | channel | yes | `0` |
| `sales/checkout/shipping_taxable` | Shipping is taxed like a product line under the shipping tax class below. Turn it off where shipping is exempt. | bool | channel |  | `true` |
| `sales/checkout/shipping_tax_class` | The tax class shipping is taxed under. Empty matches only the rules that apply to every product. | string | channel |  | - |
| `sales/checkout/risk_hold_threshold` | Orders scoring at or above this are placed on hold for manual review instead of flowing straight to fulfillment. 0 disables the hold. | int | channel |  | `0` |
| `sales/checkout/address_validator` | Which validator checks checkout addresses. Options come from what is registered, so an uninstalled provider stops being offered. An unrecognised value is treated as no checking rather than failing checkout. | select | channel | yes | `basic` |
| `sales/checkout/address_validation_strict` | Reject checkout addresses the validator marks invalid | bool | channel | yes | `false` |

**Inventory**

| Path | What it does | Type | Scope | Public | Default |
|---|---|---|---|---|---|
| `sales/inventory/ledger_enabled` | Holds stock per order line so two shoppers can't buy the last unit. Off means stock only moves on shipment, and oversells become possible. | bool | global |  | `true` |
| `sales/inventory/reservations_retention_days` | Reservation retention (days, keep well past the return window; 0 disables pruning) | int | global |  | `180` |
| `sales/inventory/default_location` | Applied when a channel is created, to seed its location assignment. Changing it does not move stock or affect channels that already exist - edit those under Inventory locations. | select | global |  | - |
| `sales/inventory/flat_mode` | The two modes store quantities in different places and nothing syncs them. Run `skelet inventory migrate-mode --to flat` (or `--to multi`) BEFORE changing this, or stock reads as zero in the new mode. | bool | global |  | `false` |
| `sales/inventory/deduction_step` | On shipment: placing an order holds stock (reservation) and the on-hand quantity drops when a shipment is created. On placement: the on-hand quantity drops the moment the order is placed - cancelling restores whatever hasn't shipped. Salable stock is identical either way; this changes what the per-location quantities mean. Multi-location mode only. | select | global |  | `fulfillment` |

**Cart**

| Path | What it does | Type | Scope | Public | Default |
|---|---|---|---|---|---|
| `sales/cart/retention_days` | How long an untouched cart survives before it is deleted. Abandoned-cart flows can't fire on a cart that is already gone. | int | global |  | `90` |

**Loyalty Program**

| Path | What it does | Type | Scope | Public | Default |
|---|---|---|---|---|---|
| `sales/loyalty/enabled` | Loyalty program enabled | bool | channel | yes | `false` |
| `sales/loyalty/earn_points_per_unit` | Points earned per currency unit of merchandise value | int | channel | yes | `1` |
| `sales/loyalty/redeem_cents_per_point` | Cents of store credit per redeemed point | int | channel | yes | `1` |

**Referral Program**

| Path | What it does | Type | Scope | Public | Default |
|---|---|---|---|---|---|
| `sales/referrals/enabled` | Referral program enabled | bool | channel | yes | `false` |
| `sales/referrals/referrer_reward_cents` | Store credit for the referrer when the referee first orders (cents) | int | channel | yes | `1000` |
| `sales/referrals/referee_reward_cents` | Store credit for the referee on their first order (cents) | int | channel | yes | `500` |

### Signals

**Analytics**

| Path | What it does | Type | Scope | Public | Default |
|---|---|---|---|---|---|
| `signals/analytics/send_log_retention_months` | Email send-log retention (months, 0 = keep forever) | int | global |  | `24` |
| `signals/analytics/spine_retention_days` | The raw per-event log behind every Signals report. Rollups outlive it, so trends survive a short window - individual events do not. | int | global |  | `365` |
| `signals/analytics/retention_action` | Prune deletes; archive writes identity-stripped copies to your backup bucket first. | select | global |  | `prune` |

**Web Analytics**

| Path | What it does | Type | Scope | Public | Default |
|---|---|---|---|---|---|
| `signals/lens/enabled` | Web analytics enabled | bool | channel |  | `true` |
| `signals/lens/sampling_rate` | Share of visitors recorded. Below 100 the totals are estimates, which is the trade for less storage on a busy store. | int | channel |  | `100` |
| `signals/lens/capture_moves` | Much the heaviest event stream - clicks and scroll depth are recorded either way. | bool | channel |  | `true` |
| `signals/lens/retention_days` | Web analytics retention (days) | int | global |  | `90` |
| `signals/lens/rollup_retention_days` | Daily traffic rollup retention (days, 0 = keep forever) | int | global |  | `0` |
| `signals/lens/replay_enabled` | Session replay enabled | bool | channel |  | `true` |
| `signals/lens/replay_sampling_rate` | Signed-in customers are recorded per their own consent; this covers everyone else. | int | channel |  | `10` |
| `signals/lens/replay_max_minutes` | Session replay max recording length (minutes; 0 = no cap) | int | channel |  | `30` |
| `signals/lens/replay_retention_days` | Session replay retention (days) | int | global |  | `30` |

**Customer Analytics**

| Path | What it does | Type | Scope | Public | Default |
|---|---|---|---|---|---|
| `signals/synapse/lifecycle_new_days` | Lifecycle: new within (days) | int | channel |  | `30` |
| `signals/synapse/lifecycle_active_days` | Lifecycle: active within (days) | int | channel |  | `90` |
| `signals/synapse/lifecycle_at_risk_days` | Lifecycle: at-risk within (days, lapsed beyond) | int | channel |  | `180` |
| `signals/synapse/return_window_days` | Engagement: return window (days) | int | channel |  | `30` |
| `signals/synapse/affinity_segment_min_actors` | Auto affinity segment: min customers per value | int | channel |  | `5` |
| `signals/synapse/affinity_segment_max_values` | Auto affinity segment: max values per attribute | int | channel |  | `20` |
| `signals/synapse/affinity_segment_window_days` | Auto affinity segment: affinity window (days) | int | channel |  | `90` |

**Deliverability**

| Path | What it does | Type | Scope | Public | Default |
|---|---|---|---|---|---|
| `signals/deliverability/auto_suppress` | Stops mailing addresses that hard-bounced or complained. Off costs sender reputation. | bool | global |  | `true` |
| `signals/deliverability/soft_bounce_threshold` | A soft bounce is temporary (full mailbox, server down), so it takes several before the address is dropped. | int | global |  | `3` |

### Outreach

**Triggers**

| Path | What it does | Type | Scope | Public | Default |
|---|---|---|---|---|---|
| `outreach/triggers/abandoned_cart_minutes` | How long a cart sits untouched before it counts as abandoned. | int | channel |  | `60` |
| `outreach/triggers/abandoned_cart_max_age_hours` | Past this the cart is too cold to chase and no flow fires - stops a backlog mailing week-old carts after downtime. | int | channel |  | `72` |
| `outreach/triggers/browse_abandoned_minutes` | Idle time after browsing with nothing added to the cart. | int | channel |  | `120` |
| `outreach/triggers/browse_abandoned_max_age_hours` | The same staleness cut-off as abandoned carts. | int | channel |  | `48` |
| `outreach/triggers/back_in_stock_min_qty` | Stops a one-unit blip mailing the whole waiting list for something that instantly resells. | int | global |  | `1` |

**Signup Forms**

| Path | What it does | Type | Scope | Public | Default |
|---|---|---|---|---|---|
| `outreach/forms/enabled` | Signup forms enabled | bool | global | yes | `true` |

### Search

**General**

| Path | What it does | Type | Scope | Public | Default |
|---|---|---|---|---|---|
| `search/general/result_limit_default` | Default result limit | int | global |  | `20` |
| `search/general/result_limit_max` | Maximum result limit | int | global |  | `100` |
| `search/general/min_query_length` | Minimum search query length | int | global |  | `1` |
| `search/general/out_of_stock_display` | Out-of-stock products in search & browse | select | global |  | `show` |

**Product Search**

| Path | What it does | Type | Scope | Public | Default |
|---|---|---|---|---|---|
| `search/products/ranking_sales_weight` | Sales-rank weight (log1p factor on 30d units; 0 = off) | string | global |  | `0.5` |
| `search/products/ranking_in_stock_boost` | In-stock score boost (flat; 0 = off) | string | global |  | `0` |
| `search/products/attributes` | Searchable attributes | search_attributes | global |  | `[{"name":"name","searchable":true,"we…` |

**Product Sort**

| Path | What it does | Type | Scope | Public | Default |
|---|---|---|---|---|---|
| `search/products_sort/options` | Sort options | sort_options | global |  | `[{"slug":"relevance","label":"Relevan…` |

**Product Listing (PLP)**

| Path | What it does | Type | Scope | Public | Default |
|---|---|---|---|---|---|
| `search/browse/page_size_default` | Products per page | int | global | yes | `24` |
| `search/browse/page_size_max` | Maximum per page | int | global |  | `100` |
| `search/browse/facet_max_values` | Max filter values per facet | int | global |  | `50` |
| `search/browse/pagination_style` | How listing pages page through products. Load more keeps every state at a real ?page=N URL, so crawlers and shared links lose nothing; hybrid shows numbered pages under the button as well. | select | global | yes | `pagination` |

**Blog Search**

| Path | What it does | Type | Scope | Public | Default |
|---|---|---|---|---|---|
| `search/blog/attributes` | Searchable attributes | search_attributes | global |  | `[{"name":"title","searchable":true,"w…` |

**Index Mapping (reindex required)**

| Path | What it does | Type | Scope | Public | Default |
|---|---|---|---|---|---|
| `search/mapping/analyzer` | Analyzer | select | global |  | `standard` |
| `search/mapping/edge_ngram_min` | Edge n-gram: min length | int | global |  | `2` |
| `search/mapping/edge_ngram_max` | Edge n-gram: max length | int | global |  | `15` |
| `search/mapping/shards` | Index shards | int | global |  | `1` |
| `search/mapping/replicas` | Index replicas | int | global |  | `0` |

### Reviews

**Submission Policy**

| Path | What it does | Type | Scope | Public | Default |
|---|---|---|---|---|---|
| `reviews/policy/require_authentication` | Require sign-in | bool | global |  | `true` |
| `reviews/policy/require_verified_purchase` | Require verified purchase | bool | global |  | `true` |

**Vote Policy**

| Path | What it does | Type | Scope | Public | Default |
|---|---|---|---|---|---|
| `reviews/votes/require_authentication` | Require sign-in to vote on reviews | bool | global |  | `true` |

### SEO

**Sitemap**

| Path | What it does | Type | Scope | Public | Default |
|---|---|---|---|---|---|
| `seo/sitemap/include_products` | Include products in the sitemap | bool | channel | yes | `true` |
| `seo/sitemap/include_categories` | Include categories in the sitemap | bool | channel | yes | `true` |
| `seo/sitemap/include_collections` | Include collections in the sitemap | bool | channel | yes | `true` |
| `seo/sitemap/include_cms` | Include CMS pages in the sitemap | bool | channel | yes | `true` |
| `seo/sitemap/include_blog` | Include blog posts in the sitemap | bool | channel | yes | `true` |
| `seo/sitemap/max_urls_per_file` | Catalog feeds split into numbered files of at most this many URLs (the protocol caps a file at 50,000). | int | channel | yes | `45000` |
| `seo/sitemap/exclude_paths` | Exclude paths (one prefix per line) | textarea | channel | yes | - |
| `seo/sitemap/extra_urls` | Extra URLs (one per line) | textarea | channel | yes | - |

**robots.txt**

| Path | What it does | Type | Scope | Public | Default |
|---|---|---|---|---|---|
| `seo/robots/robots_txt` | robots.txt (blank = managed default) | textarea | channel | yes | - |

**Facet indexing**

| Path | What it does | Type | Scope | Public | Default |
|---|---|---|---|---|---|
| `seo/facets/indexable_combinations` | Filter pages combining several facets are indexable only when the combination is listed here - one per line, slugs joined by +. | textarea | channel | yes | - |
| `seo/facets/min_results` | Filter pages with fewer matching products go noindex; 0 disables the guard. | int | channel | yes | `0` |
| `seo/facets/robots_noindex` | Keep follow so crawlers still reach the products linked from the page. | select | channel | yes | `noindex,follow` |
| `seo/facets/robots_indexable` | Omitting the tag means index, follow - emit it explicitly only if an audit asks for it. | select | channel | yes | `omit` |
| `seo/facets/max_indexable_page` | Filter pages past this depth go noindex, so facets and pagination can't multiply into a crawl trap. 1 indexes a filtered listing's first page only; 0 removes the cap. | int | channel | yes | `1` |
| `seo/facets/max_indexable_page_unfiltered` | The same cap for listings with no filter applied. 0 removes the cap, so the whole pager stays indexable and deep products keep a route into the index. | int | channel | yes | `0` |

**Site crawl**

| Path | What it does | Type | Scope | Public | Default |
|---|---|---|---|---|---|
| `seo/crawl/nightly_enabled` | Crawl this storefront nightly | bool | channel |  | `false` |
| `seo/crawl/max_pages` | The crawl stops here, so a big catalogue is covered across nights rather than in one run. | int | channel |  | `2000` |
| `seo/crawl/alert_threshold` | Broken links and redirect chains new since the last crawl. | int | channel |  | `1` |

### CMS

**Editorial Workflow**

| Path | What it does | Type | Scope | Public | Default |
|---|---|---|---|---|---|
| `cms/workflow/require_approval` | Require approval before publishing | bool | global |  | `false` |

**Custom Forms**

| Path | What it does | Type | Scope | Public | Default |
|---|---|---|---|---|---|
| `cms/forms/enabled` | Enable custom forms on the storefront | bool | global |  | `true` |
| `cms/forms/captcha_provider` | CAPTCHA provider for forms that require it | select | global |  | `none` |
| `cms/forms/captcha_site_key` | CAPTCHA site key (public) | string | global |  | - |
| `cms/forms/captcha_secret_key` | CAPTCHA secret key | secret | global |  | - |

### Payment

**Bank Transfer / Invoice**

| Path | What it does | Type | Scope | Public | Default |
|---|---|---|---|---|---|
| `payment/bank-transfer/instructions` | Bank Transfer / Invoice - payment instructions shown to the customer | string | channel |  | `We'll email you our bank details. Ref…` |

**Cash on Delivery**

| Path | What it does | Type | Scope | Public | Default |
|---|---|---|---|---|---|
| `payment/cod/instructions` | Cash on Delivery - payment instructions shown to the customer | string | channel |  | `Pay with cash when your order is deli…` |

**Pay on Pickup**

| Path | What it does | Type | Scope | Public | Default |
|---|---|---|---|---|---|
| `payment/pay-on-pickup/instructions` | Pay on Pickup - payment instructions shown to the customer | string | channel |  | `Reserve now and pay in store when you…` |

**Purchase Order**

| Path | What it does | Type | Scope | Public | Default |
|---|---|---|---|---|---|
| `payment/purchase-order/instructions` | Purchase Order - payment instructions shown to the customer | string | channel |  | `Enter your PO number in the order not…` |

### AI

**LLM provider**

| Path | What it does | Type | Scope | Public | Default |
|---|---|---|---|---|---|
| `ai/llm/enabled` | Enable AI content assistant | bool | global |  | `false` |
| `ai/llm/provider` | Provider | select | global |  | `anthropic` |
| `ai/llm/api_key` | API key | secret | global |  | - |
| `ai/llm/model` | Model (e.g. claude-opus-5-5, claude-sonnet-5-5, gpt-4o, gemini-2.0-flash) | string | global |  | `claude-opus-5-5` |
| `ai/llm/vision_model` | Vision model for alt-text (blank = use Model) | string | global |  | - |
| `ai/llm/base_url` | API base URL (blank = provider default; set for Azure/OpenRouter/Ollama/local) | string | global |  | - |
| `ai/llm/tools` | Let the assistant query data via tools (turn off for models without tool/function calling) | bool | global |  | `true` |
| `ai/llm/allow_customer_data` | Allow the assistant to read individual customer data (profiles, records). Turn off to keep personal data out of the AI provider. | bool | global |  | `true` |
| `ai/llm/history_retention_days` | Chat history retention (days; 0 = keep forever) | int | global |  | `90` |

### Cache

**Navigation (header, menus, currencies)**

| Path | What it does | Type | Scope | Public | Default |
|---|---|---|---|---|---|
| `cache/nav/enabled` | Navigation (header, menus, currencies) - cached | bool | global | yes | `true` |
| `cache/nav/ttl_seconds` | Navigation (header, menus, currencies) - TTL (seconds) | int | global | yes | `3600` |

**Products (PDP)**

| Path | What it does | Type | Scope | Public | Default |
|---|---|---|---|---|---|
| `cache/product/enabled` | Products (PDP) - cached | bool | global | yes | `true` |
| `cache/product/ttl_seconds` | Products (PDP) - TTL (seconds) | int | global | yes | `3600` |
| `cache/product/mode` | On product change | select | global |  | `invalidate` |
| `cache/product/on_update` | …when a product is updated (incl. price) | bool | global |  | `true` |
| `cache/product/on_stock_change` | …when stock changes | bool | global |  | `true` |
| `cache/product/on_delete` | …when a product is deleted (always invalidates) | bool | global |  | `true` |

**Catalog listings (categories, grids, collections)**

| Path | What it does | Type | Scope | Public | Default |
|---|---|---|---|---|---|
| `cache/catalog/enabled` | Catalog listings (categories, grids, collections) - cached | bool | global | yes | `true` |
| `cache/catalog/ttl_seconds` | Catalog listings (categories, grids, collections) - TTL (seconds) | int | global | yes | `3600` |

**CMS pages**

| Path | What it does | Type | Scope | Public | Default |
|---|---|---|---|---|---|
| `cache/cms/enabled` | CMS pages - cached | bool | global | yes | `true` |
| `cache/cms/ttl_seconds` | CMS pages - TTL (seconds) | int | global | yes | `3600` |

### System

**Scheduled jobs**

| Path | What it does | Type | Scope | Public | Default |
|---|---|---|---|---|---|
| `system/scheduler/enabled` | Scheduler enabled (master pause switch) | bool | global |  | `true` |
| `system/scheduler/alert_webhook_url` | Failure alert webhook URL | string | global |  | - |
| `system/scheduler/history_retention_days` | Run-history retention (days) | int | global |  | `30` |

**Audit log**

| Path | What it does | Type | Scope | Public | Default |
|---|---|---|---|---|---|
| `system/audit/retention_days` | Audit log retention (days) | int | global |  | `365` |

**Notifications**

| Path | What it does | Type | Scope | Public | Default |
|---|---|---|---|---|---|
| `system/notifications/retention_days` | Notification retention after read (days) | int | global |  | `30` |

**Import & export**

| Path | What it does | Type | Scope | Public | Default |
|---|---|---|---|---|---|
| `system/data_transfer/artifact_retention_days` | How long an export stays downloadable and an import's source file is kept for re-reading. The job history outlives the file | int | global |  | `7` |
| `system/data_transfer/history_retention_days` | How long finished import and export runs stay listed | int | global |  | `90` |

**Webhooks**

| Path | What it does | Type | Scope | Public | Default |
|---|---|---|---|---|---|
| `system/webhooks/delivery_retention_days` | Webhook delivery-log retention (days) | int | global |  | `90` |
<!-- END GENERATED SETTINGS -->
