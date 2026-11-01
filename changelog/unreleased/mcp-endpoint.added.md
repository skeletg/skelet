- **AI agents can read the store over MCP.** `/api/admin/mcp` serves the dashboard chat's tools to
  any MCP client that can send an integration token in `X-Api-Token`, Claude Code included. It's
  read-only, reads one channel per connection, and offers only the tools the token's permissions
  cover - sales, catalog, orders, stock, promotions, reviews and store setup. Each token can run 4
  calls at once and, with `RATE_LIMIT_ENABLED` on, `MCP_RATE_LIMIT_PER_MIN` a minute.
  `MCP_ENABLED=false` turns it off. Setup is in `docs/mcp.md`.
