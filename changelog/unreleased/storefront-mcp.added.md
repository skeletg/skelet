- **Shoppers' AI agents can shop the store.** Every storefront answers MCP at `/api/mcp`, so a
  shopper's assistant can search the catalog, read product details, check delivery and build a
  cart. It doesn't check out: `start_checkout` gives the shopper a link that copies the cart into
  their own, and they review it and pay on the store. `sales/agents/mcp_enabled` turns it off for a
  channel, `MCP_FRONT_ENABLED=false` everywhere.
