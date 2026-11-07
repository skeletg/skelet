- **A shopping assistant on the storefront.** With `sales/agents/assistant_enabled` on, Vetrina shows a
  sparkles button in its header that opens a full-height panel like the dashboard's chat, or a drawer on a
  phone. It answers through the store's AI provider using the storefront MCP tools: it shows the products
  it suggests as cards with an add to cart button and the one it explains as a panel of its specs, points
  to narrowed listings and pages with buttons, finds the current deals, knows the page the shopper is on
  and refilters or resorts the list they're browsing, and fills the shopper's own cart as
  they ask, so the header updates at once and a Review cart button opens the cart drawer. Off by default,
  since every message is a paid model call, and capped per store per day
  (`sales/agents/assistant_daily_turns`) and per shopper (`sales/agents/assistant_per_minute`, on even
  without `RATE_LIMIT_ENABLED`). A signed-in shopper can ask where an order is, what was in it and how a
  return is going, and start a return - which the assistant sends only after the shopper confirms its
  summary in a later message.
