# Shoppers' agents over MCP

Every storefront answers [MCP](https://modelcontextprotocol.io) at `/api/mcp`, so a shopper can ask
their own AI assistant to find something on your store and fill a cart with it. The agent searches,
compares, checks delivery and builds the cart. It never places the order: when the shopper is ready
it hands them a link, and they finish on the store like anyone else.

This is a different endpoint from the one in [mcp.md](mcp.md). That one reads your sales and orders
for your own team, behind a token. This one is public, anonymous, and sees what any visitor sees.

## Connecting

The address is the storefront's own: `https://shop.example.com/api/mcp`. There's no sign-in, so a
client that asks for authentication should be told "none". In Claude Code:

```bash
claude mcp add --transport http shop https://shop.example.com/api/mcp
```

The storefront forwards the call to core's `/api/front/mcp` and names its own channel, the same way it
does for GraphQL, so an agent can only shop the channel whose address it used. It sends nothing of the
caller's along: no cookies, no session. A channel needs its public URL set before agents can use it,
since every link they hand on is built from it.

Storefronts forked from a template before this shipped don't have the `/api/mcp` and `/handoff` routes.
Copy `app/api/mcp/route.ts` and `app/handoff/[token]/route.ts` from the template (or the Astro pair under
`src/pages/`), or turn the endpoint off for that channel.

## What an agent can do

| Tool | What it does |
|---|---|
| `get_store_info` | Name, currency, languages, countries, sort orders, free-shipping threshold |
| `search_products` | Search and filter the catalog, with facets the agent can narrow by |
| `get_product` | One product by id, SKU or link: price, stock, details, variants, reviews, and which SKU goes in the cart |
| `list_categories` | The category tree, a level at a time |
| `search_store_pages` | Your CMS pages, for shipping, returns, sizing and the like |
| `estimate_shipping` | Delivery options and prices to a country, for a cart if it has one |
| `get_cart` | What's in the cart it built |
| `update_cart` | Start a cart, or set SKUs in it to exact quantities, and apply a code |
| `start_checkout` | Close the cart and get the link for the shopper |

Every read goes through the storefront's own GraphQL resolvers, in-process, so prices, stock, private
attributes, search merchandising and promotions come out exactly as the storefront shows them. Cart
edits go to the cart service directly, with the same checks the storefront's own cart calls make.
`search_store_pages` keeps each channel's page text for a minute, so a page edit can take that long to
show up there.

Products that need choosing on the page don't go in from here: bundles, gift cards, subscriptions and
anything with required options. `get_product` says so and gives the agent the link instead.
Configurable products go in by variant SKU, and grouped ones by their members' SKUs.

## The handoff

An agent's cart is a cart like any other, marked as built by an agent. Its id lives in the agent's
conversation, on someone else's servers, so it's never the cart the shopper checks out with.
`start_checkout` closes it and returns a link to `/handoff/<token>`. Opening the link copies the
lines into the shopper's own cart: the signed-in customer's, else the guest cart their browser already
has, else a new one. Then it lands them on the cart to look it over.

- An agent cart answers only the MCP tools. The storefront's GraphQL API treats its id like any other
  cart that isn't yours, so the link is the only way from an agent's cart to a checkout.
- A closed agent cart can't be read or changed again, by the agent or anyone else.
- The link lasts 7 days and can be opened more than once. Opening it again into the same cart adds
  nothing, so a reload or a chat app previewing the link can't double the order. That holds after a
  guest signs in and their cart merges into their account's, too. A preview does leave an extra guest
  cart behind, which the cart cleanup takes care of.
- Prices are whatever the cart says at checkout, not what the agent saw.

## Turning it off

`sales/agents/mcp_enabled` turns it off for a channel (see [settings.md](settings.md)), and
`MCP_FRONT_ENABLED=false` takes the endpoint away entirely.

## Limits and logs

Hosted assistants call from a handful of shared addresses, so the budgets per client address are wide:
once `RATE_LIMIT_ENABLED` is on, `MCP_FRONT_RATE_LIMIT_PER_MIN` tool calls a minute (600 by default),
and always `MCP_FRONT_MAX_IN_FLIGHT` calls at once (32). A call gets 20 seconds. Every call is logged as
`mcp: tool` with the tool, the channel, the client address and whether it failed.

The client address comes from `X-Forwarded-For`, which the storefront passes on as it gets it. Keep an
edge like the bundled Caddy in front of the storefront, so what a client sends there gets replaced
rather than trusted.

Reviews come back quoted, and the agent is told they were written by other shoppers. A review can
still try to talk an agent into something, which is one more reason the agent never pays: whatever it
put in the cart, the shopper sees it before buying.
