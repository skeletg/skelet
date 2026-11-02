# Reading the store over MCP

The backend serves an [MCP](https://modelcontextprotocol.io) endpoint, so an agent like Claude Code
can answer questions about your store from its own data: how sales are trending, what's low on
stock, what a given order looks like. It's read-only. Nothing an agent does through it changes the
store.

The tools are the ones the dashboard's AI chat uses, gated the same way, so an agent sees exactly
what the credential it connects with may see. The endpoint sits beside admin GraphQL:

```
/api/admin/mcp
/api/admin/<ADMIN_API_SLUG>/mcp     # when you've set ADMIN_API_SLUG
```

It answers wherever admin GraphQL does - on the dashboard host, whose nginx forwards `/api/*`, and on
the API host. The app prints the exact URL at boot as `admin MCP endpoint`. `MCP_ENABLED=false`
takes the endpoint away entirely.

## Connecting an agent

1. In the dashboard, open **Developer → Integrations** and create a token. Give it the `.view`
   permissions you want the agent to have and nothing else, and pin it to the channel it should
   read. The raw token is shown once.
2. Point your MCP client at the endpoint with the token in the `X-Api-Token` header. In Claude Code:

   ```bash
   claude mcp add --transport http skelet https://admin.example.com/api/admin/mcp \
     --header "X-Api-Token: sk_..."
   ```

   Or in a client's JSON config:

   ```json
   {
     "mcpServers": {
       "skelet": {
         "type": "http",
         "url": "https://admin.example.com/api/admin/mcp",
         "headers": { "X-Api-Token": "sk_..." }
       }
     }
   }
   ```

Any client that can send a request header works. A signed-in admin's session token works too, as
`Authorization: Bearer`, but it expires within minutes, so it's only useful for trying things out.
Clients that only speak OAuth, like most hosted connectors, can't connect yet - the endpoint has no
OAuth flow.

## One channel per connection

A connection reads one channel. A pinned token reads its own. An unpinned one has to name a channel
with `X-Channel-Id: <channel id>`, and a store with several channels gets one MCP server entry per
channel. Sales, orders, customers, stock, promotions, subscriptions, gift cards, shipping, tax and
redirects answer for that channel. The catalog, CMS pages, menus, currencies and locales are
store-wide, the same as they are in the dashboard.

The endpoint refuses before any tool runs when no channel is named or the header isn't a channel id
(400), and when the credential can't read anything on that channel (403). A channel that doesn't
exist gets the same 403, so the endpoint can't be used to find out which channels do.

## Which tools show up

The tool list is built per request from the credential's permissions on that channel, so an agent
never sees a tool it can't call:

| Permission | Tools |
|---|---|
| `reports.view` | sales summary, trend, top products, refunds |
| `catalog.view` | product search and lookup, categories, collections, product types, attributes |
| `orders.view` | order search and lookup |
| `customers.view` | customer lookup |
| `inventory.view` | stock for a product, low-stock list |
| `promotions.view` | promotions and how they performed |
| `reviews.view` | a product's reviews |
| `subscriptions.view`, `giftcards.view` | subscriptions, gift cards and code lookups |
| `channels.view`, `currencies.view`, `locales.view` | store setup |
| `settings.view` | shipping methods, tax rules |
| `cms.view`, `urlrewrite.view` | menus, CMS pages, redirects |

Two things narrow it further:

- **Signals tools need a signed-in admin.** Traffic, customer behaviour, experiments and email
  engagement live in the Signals service, which only accepts tokens minted for an admin session,
  so an integration token doesn't get them.
- **Customer data follows `ai/llm/allow_customer_data`.** Turn it off and the per-customer tools
  drop out of the list and the order tools stop showing customer emails, for MCP and the dashboard
  chat alike.

Tools that take `window_days` look back 30 days when the agent leaves it out.

## Text written by shoppers and visitors

Some of what the tools return was written by people outside your team. Shoppers write their names,
their emails and the reviews behind a product's review summary. Anyone visiting the store, signed in
or not, decides what they search for, which addresses they open, the referrer they arrive with and
the custom events and errors their browser reports. The server quotes all of that and tells the
agent to treat it as data, but an agent is still a language model reading it, so a crafted search or
review could try to talk it into something. Keep your client's approval prompts on for tools that act - shell commands, file
edits, other servers - in sessions where this server is connected.

## Limits and logs

Each credential gets `MCP_RATE_LIMIT_PER_MIN` tool calls a minute (120 by default) once
`RATE_LIMIT_ENABLED` is on - see [env.md](env.md) - and at most 4 calls running at once, whatever
that setting says. Past either limit, the call comes back as a tool error the agent can read and
back off from, not a dropped connection. Batched JSON-RPC requests are refused, one message per
request.

Every call is logged as `mcp: tool` with the tool name, the credential's kind and id, the channel
and whether it failed. A failed read also logs its cause, and a tool that panics is caught, reported
like any other server panic, and answered as a tool error. Since nothing writes, nothing lands in
the audit log.
