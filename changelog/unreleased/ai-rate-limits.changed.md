- **Everything on the storefront that reaches an AI provider is rate limited, per store, in the settings.** Search by
  meaning limits each shopper to `search/products/meaning_per_minute` new questions a minute (60; a question already
  asked is free, and past it the search runs on its words), so one visitor can't spend the provider for everyone. The
  storefront MCP endpoint's limit moved from `MCP_FRONT_RATE_LIMIT_PER_MIN` to `sales/agents/mcp_per_minute` (600) and
  is now on whatever `RATE_LIMIT_ENABLED` says. A whole IPv6 /64 counts as one shopper. 0 turns any of them off.
