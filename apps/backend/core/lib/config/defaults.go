package config

// Fields already living as channels-table columns are NOT registered here - the store form owns those

func init() {
	RegisterSection(Section{Slug: "general", Label: "General", SortOrder: 1})
	RegisterSection(Section{Slug: "sales", Label: "Sales", SortOrder: 2})
	RegisterSection(Section{Slug: "signals", Label: "Signals", SortOrder: 3})
	RegisterSection(Section{Slug: "outreach", Label: "Outreach", SortOrder: 4})
	RegisterSection(Section{Slug: "seo", Label: "SEO", SortOrder: 5})

	RegisterGroup(Group{Section: "general", Slug: "store_information", Label: "Store Information", SortOrder: 1, Scoping: ScopingChannel})
	RegisterGroup(Group{Section: "general", Slug: "security", Label: "Security", SortOrder: 2})
	RegisterGroup(Group{Section: "general", Slug: "urls", Label: "URLs & Redirects", SortOrder: 3})
	RegisterGroup(Group{Section: "sales", Slug: "checkout", Label: "Checkout", SortOrder: 1, Scoping: ScopingChannel})
	RegisterGroup(Group{Section: "sales", Slug: "orders", Label: "Orders", SortOrder: 2, Scoping: ScopingChannel})
	RegisterGroup(Group{Section: "sales", Slug: "inventory", Label: "Inventory", SortOrder: 3})
	RegisterGroup(Group{Section: "sales", Slug: "cart", Label: "Cart", SortOrder: 4})
	RegisterGroup(Group{Section: "signals", Slug: "analytics", Label: "Analytics", SortOrder: 1})
	RegisterGroup(Group{Section: "signals", Slug: "lens", Label: "Web Analytics", SortOrder: 2, Scoping: ScopingChannel})
	RegisterGroup(Group{Section: "signals", Slug: "synapse", Label: "Customer Analytics", SortOrder: 3, Scoping: ScopingChannel})
	RegisterGroup(Group{Section: "signals", Slug: "deliverability", Label: "Deliverability", SortOrder: 4})
	// Core's own: when the store reaches out to a customer nobody asked it to mail
	RegisterGroup(Group{Section: "outreach", Slug: "triggers", Label: "Triggers", SortOrder: 1})
	RegisterGroup(Group{Section: "outreach", Slug: "forms", Label: "Signup Forms", SortOrder: 2})

	RegisterGroup(Group{Section: "seo", Slug: "sitemap", Label: "Sitemap", SortOrder: 1, Scoping: ScopingChannel})
	RegisterGroup(Group{Section: "seo", Slug: "robots", Label: "robots.txt", SortOrder: 2, Scoping: ScopingChannel})
	RegisterGroup(Group{Section: "seo", Slug: "facets", Label: "Facet indexing", SortOrder: 3, Scoping: ScopingChannel})
	RegisterGroup(Group{Section: "seo", Slug: "crawl", Label: "Site crawl", SortOrder: 4, Scoping: ScopingChannel})

	Register(Definition{Path: "general/store_information/support_phone", Type: TypeString, Default: "", Label: "Store phone number", Description: "Served to storefronts over publicConfig; the bundled one doesn't show it.", Public: true})
	Register(Definition{Path: "general/store_information/description", Type: TypeTextarea, Default: "", Label: "Store description", Description: "A short line about the store. Served to storefronts over publicConfig; the bundled one doesn't show it.", Public: true})
	Register(Definition{Path: "general/store_information/phone_mobile", Type: TypePhone, Default: "", Label: "Mobile phone", Description: "A second number, for stores that publish one. Served to storefronts; the bundled one doesn't show it.", Public: true})
	// Options come from the currencies the operator actually created, not a hardcoded three - and the label says what the value does: it is the base the currency-rates job quotes every other rate against, NOT a display preference. Path kept as-is so existing rows keep resolving
	Register(Definition{Path: "general/store_information/currency_list", Scoping: ScopingGlobal, Type: TypeSelect, Default: "USD", Label: "Base currency for exchange rates", Description: "The currency every stored exchange rate is quoted against, refreshed twice daily from ECB. Storefront prices come from each channel's own currency, not from here.", Public: true, Source: "currencies"})
	// Replaced a free-text "hours" string that nothing parsed - this one is per-day ranges, so a storefront or LocalBusiness JSON-LD block can actually use it
	Register(Definition{Path: "general/store_information/working_hours", Type: TypeWorkingHours, Default: "{}", Label: "Opening hours", Description: "Per-day opening times. Served to storefronts over publicConfig; the bundled one doesn't show them.", Public: true})

	Register(Definition{Path: "general/security/admin_single_session", Type: TypeBool, Default: "false", Label: "Log out other admin devices on new login", Description: "Signing in revokes every other admin session for that account. Stops a shared login being used from two places, at the cost of forcing a re-login when someone moves between machines.", Public: false})
	Register(Definition{Path: "general/security/catalog_query_throttle", Scoping: ScopingChannel, Type: TypeInt, Default: "0", Label: "Catalog query throttle: per-IP soft budget (req/min, 0 = off; slows past budget, refuses past 3x)", Description: "Soft per-IP budget for storefront catalog queries. Past the budget requests are slowed rather than refused; past three times it they are refused outright. 0 disables both.", Public: false})

	Register(Definition{Path: "general/urls/auto_redirect_on_change", Scoping: ScopingGlobal, Type: TypeBool, Default: "true", Label: "Create a redirect when a URL changes", Description: "Keeps old links working after a slug edit. Off means the previous URL 404s.", Public: false})
	Register(Definition{Path: "general/urls/redirect_type", Type: TypeSelect, Default: "301", Label: "Automatic redirect type", Public: false, Options: []Option{
		{Value: "301", Label: "301 (permanent)"},
		{Value: "302", Label: "302 (temporary)"},
	}})
	Register(Definition{Path: "general/urls/url_template_fallback", Scoping: ScopingGlobal, Type: TypeBool, Default: "true", Label: "Fall back to a slug when a URL template can't render", Description: "Off means the save is rejected instead, which catches a broken template rather than papering over it.", Public: false})
	Register(Definition{Path: "general/urls/redirect_retention_days", Type: TypeInt, Default: "365", Label: "Auto redirect retention (days, 0 = keep forever)", Description: "Only auto-created redirects are pruned - ones you added by hand are kept.", Public: false})
	Register(Definition{Path: "general/urls/variant_own_url", Scoping: ScopingChannel, Type: TypeBool, Default: "true", Label: "Give configurable variants their own URLs", Description: "Each variant becomes its own indexable page. More pages to crawl, but a shopper landing from search arrives on the exact one.", Public: true})

	Register(Definition{Path: "sales/checkout/allow_guest", Type: TypeBool, Default: "true", Label: "Allow guest checkout", Description: "Whether someone can complete checkout without an account. Off forces registration or sign-in before the order can be placed.", Public: true})
	Register(Definition{Path: "sales/checkout/min_order", Type: TypeInt, Default: "0", Label: "Minimum order amount (cents)", Description: "Orders below this merchandise total are refused at checkout. In minor units, so 2500 is $25.00. 0 disables the floor.", Public: true})
	Register(Definition{Path: "sales/checkout/shipping_taxable", Type: TypeBool, Default: "true", Label: "Charge tax on shipping", Description: "Shipping is taxed like a product line under the shipping tax class below. Turn it off where shipping is exempt."})
	Register(Definition{Path: "sales/checkout/shipping_tax_class", Type: TypeString, Default: "", Label: "Shipping tax class", Description: "The tax class shipping is taxed under. Empty matches only the rules that apply to every product."})
	Register(Definition{Path: "sales/checkout/risk_hold_threshold", Type: TypeInt, Default: "0", Label: "Auto-hold orders at or above this risk score (0 = off)", Description: "Orders scoring at or above this are placed on hold for manual review instead of flowing straight to fulfillment. 0 disables the hold.", Public: false})

	// ledger_enabled=false short-circuits reservation writes (ERP-owned inventory / cutover)
	Register(Definition{Path: "sales/inventory/ledger_enabled", Type: TypeBool, Default: "true", Label: "Reservation ledger enabled", Description: "Holds stock per order line so two shoppers can't buy the last unit. Off means stock only moves on shipment, and oversells become possible.", Public: false})
	// Pruned chains carry the shipped-from cells return restocking reads AND the per-return replay markers,
	// so retention must comfortably exceed the return window - a return against a pruned order restocks nothing, silently
	Register(Definition{Path: "sales/inventory/reservations_retention_days", Type: TypeInt, Default: "180", Label: "Reservation retention (days, keep well past the return window; 0 disables pruning)", Public: false})
	Register(Definition{Path: "sales/cart/retention_days", Type: TypeInt, Default: "90", Label: "Cart retention (days)", Description: "How long an untouched cart survives before it is deleted. Abandoned-cart flows can't fire on a cart that is already gone.", Public: false})

	// whole months only (the send log's partition unit) - the log lives in the commerce database and every transactional
	// mail writes a row, so it ages out by default rather than growing forever on a store that never opens Signals

	Register(Definition{Path: "outreach/triggers/abandoned_cart_minutes", Scoping: ScopingChannel, Type: TypeInt, Default: "60", Label: "Abandoned-cart idle threshold (minutes)", Description: "How long a cart sits untouched before it counts as abandoned.", Public: false})
	Register(Definition{Path: "outreach/triggers/abandoned_cart_max_age_hours", Scoping: ScopingChannel, Type: TypeInt, Default: "72", Label: "Abandoned-cart max age (hours)", Description: "Past this the cart is too cold to chase and no flow fires - stops a backlog mailing week-old carts after downtime.", Public: false})
	Register(Definition{Path: "outreach/triggers/browse_abandoned_minutes", Scoping: ScopingChannel, Type: TypeInt, Default: "120", Label: "Browse-abandonment idle threshold (minutes)", Description: "Idle time after browsing with nothing added to the cart.", Public: false})
	Register(Definition{Path: "outreach/triggers/browse_abandoned_max_age_hours", Scoping: ScopingChannel, Type: TypeInt, Default: "48", Label: "Browse-abandonment max age (hours)", Description: "The same staleness cut-off as abandoned carts.", Public: false})
	// min salable units to treat a SKU as back (avoids firing on a 1-unit blip)
	Register(Definition{Path: "outreach/triggers/back_in_stock_min_qty", Type: TypeInt, Default: "1", Label: "Back-in-stock: minimum salable units to notify", Description: "Stops a one-unit blip mailing the whole waiting list for something that instantly resells.", Public: false})
	Register(Definition{Path: "outreach/forms/enabled", Type: TypeBool, Default: "true", Label: "Signup forms enabled", Public: true})
	// Read once, in channel.Create - it seeds a NEW channel's location assignment and has no effect on existing channels, orders or stock reads
	Register(Definition{Path: "sales/inventory/default_location", Type: TypeSelect, Default: "", Label: "Inventory location for new channels", Description: "Applied when a channel is created, to seed its location assignment. Changing it does not move stock or affect channels that already exist - edit those under Inventory locations.", Public: false, Source: "inventory_locations"})
	// dormant multi-location paths; reads item.quantity, ERP-synced - does NOT migrate data on flip
	// The two modes read different columns and nothing keeps them in step, so flipping without carrying the stock across reads the whole catalogue as out of stock
	Register(Definition{Path: "sales/inventory/flat_mode", Type: TypeBool, Default: "false", Label: "Flat inventory mode (single qty per product, no warehouses)", Description: "The two modes store quantities in different places and nothing syncs them. Run `skelet inventory migrate-mode --to flat` (or `--to multi`) BEFORE changing this, or stock reads as zero in the new mode.", Public: false})
	// Steers only NEW placements - cancel/edit/return read the ledger for what actually happened, so flipping with orders in flight stays consistent. Flat mode always deducts at placement and ignores this
	Register(Definition{Path: "sales/inventory/deduction_step", Type: TypeSelect, Default: "fulfillment", Label: "When on-hand stock is deducted", Description: "On shipment: placing an order holds stock (reservation) and the on-hand quantity drops when a shipment is created. On placement: the on-hand quantity drops the moment the order is placed - cancelling restores whatever hasn't shipped. Salable stock is identical either way; this changes what the per-location quantities mean. Multi-location mode only.", Public: false, Options: []Option{
		{Value: "fulfillment", Label: "On shipment (fulfillment)"},
		{Value: "placement", Label: "On order placement"},
	}})

	// Public + read per-channel by the storefront's sitemap/robots routes - each toggle drops a whole feed from the sitemap index
	Register(Definition{Path: "seo/sitemap/include_products", Type: TypeBool, Default: "true", Label: "Include products in the sitemap", Public: true})
	Register(Definition{Path: "seo/sitemap/include_categories", Type: TypeBool, Default: "true", Label: "Include categories in the sitemap", Public: true})
	Register(Definition{Path: "seo/sitemap/include_collections", Type: TypeBool, Default: "true", Label: "Include collections in the sitemap", Public: true})
	Register(Definition{Path: "seo/sitemap/include_cms", Type: TypeBool, Default: "true", Label: "Include CMS pages in the sitemap", Public: true})
	Register(Definition{Path: "seo/sitemap/include_blog", Type: TypeBool, Default: "true", Label: "Include blog posts in the sitemap", Public: true})
	// The sitemap spec caps a file at 50k URLs - the storefront clamps whatever is stored here to 1..50000
	Register(Definition{Path: "seo/sitemap/max_urls_per_file", Type: TypeInt, Default: "45000", Label: "Max URLs per sitemap file", Description: "Catalog feeds split into numbered files of at most this many URLs (the protocol caps a file at 50,000).", Public: true})
	// The facet-indexing group is managed by the dashboard's dedicated /seo/facets page and hidden from the generic settings UI - single-facet pages index per attribute/value flags, multi-facet pages only when their combination is listed, so crawlers never see the 2^n loop
	Register(Definition{Path: "seo/facets/indexable_combinations", Type: TypeTextarea, Default: "", Label: "Indexable filter combinations", Description: "Filter pages combining several facets are indexable only when the combination is listed here - one per line, slugs joined by +.", Public: true})
	Register(Definition{Path: "seo/facets/min_results", Type: TypeInt, Default: "0", Label: "Min results for an indexable filter page", Description: "Filter pages with fewer matching products go noindex; 0 disables the guard.", Public: true})
	// What the listing pages actually put in their robots meta - the rules above decide WHETHER a
	// filter page indexes, these two decide how that verdict is phrased to a crawler
	Register(Definition{Path: "seo/facets/robots_noindex", Type: TypeSelect, Default: "noindex,follow", Label: "Robots tag on non-indexable filter pages", Description: "Keep follow so crawlers still reach the products linked from the page.", Public: true, Options: []Option{
		{Value: "noindex,follow", Label: "noindex, follow"},
		{Value: "noindex,nofollow", Label: "noindex, nofollow"},
	}})
	Register(Definition{Path: "seo/facets/robots_indexable", Type: TypeSelect, Default: "omit", Label: "Robots tag on indexable listing pages", Description: "Omitting the tag means index, follow - emit it explicitly only if an audit asks for it.", Public: true, Options: []Option{
		{Value: "omit", Label: "No tag (implicit index, follow)"},
		{Value: "index,follow", Label: "index, follow"},
	}})
	// Filters multiply with pages: every indexable facet value times every page is another near-duplicate URL, which is how a listing turns into an unbounded crawl space. These cap how deep each axis may be indexed - 0 lifts the cap. Capped pages stay noindex,FOLLOW, since under load-more paging the pager links are the only route to deep products
	Register(Definition{Path: "seo/facets/max_indexable_page", Type: TypeInt, Default: "1", Label: "Deepest indexable page of a filtered listing", Description: "Filter pages past this depth go noindex, so facets and pagination can't multiply into a crawl trap. 1 indexes a filtered listing's first page only; 0 removes the cap.", Public: true})
	Register(Definition{Path: "seo/facets/max_indexable_page_unfiltered", Type: TypeInt, Default: "0", Label: "Deepest indexable page of an unfiltered listing", Description: "The same cap for listings with no filter applied. 0 removes the cap, so the whole pager stays indexable and deep products keep a route into the index.", Public: true})

	Register(Definition{Path: "seo/sitemap/exclude_paths", Type: TypeTextarea, Default: "", Label: "Exclude paths (one prefix per line)", Public: true})
	// one absolute or root-relative URL per line, appended verbatim to the root sitemap
	Register(Definition{Path: "seo/sitemap/extra_urls", Type: TypeTextarea, Default: "", Label: "Extra URLs (one per line)", Public: true})
	// blank = the managed default (baseline disallows + the sitemap link) - a non-blank value is served verbatim as the whole file
	Register(Definition{Path: "seo/robots/robots_txt", Type: TypeTextarea, Default: "", Label: "robots.txt (blank = managed default)", Public: true})

	Register(Definition{Path: "seo/crawl/nightly_enabled", Type: TypeBool, Default: "false", Label: "Crawl this storefront nightly"})
	Register(Definition{Path: "seo/crawl/max_pages", Type: TypeInt, Default: "2000", Label: "Nightly crawl page budget", Description: "The crawl stops here, so a big catalogue is covered across nights rather than in one run."})
	// A run that only shifts a handful of pages is normal churn - the alert is for the day something breaks at scale
	Register(Definition{Path: "seo/crawl/alert_threshold", Type: TypeInt, Default: "1", Label: "Notify when a nightly crawl finds at least this many regressions", Description: "Broken links and redirect chains new since the last crawl."})
}
