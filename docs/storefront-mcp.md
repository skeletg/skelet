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
caller's along: no cookies, no session.

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
attributes, search merchandising and promotions come out exactly as the storefront shows them.

Products that need choosing on the page don't go in from here: bundles, gift cards, subscriptions and
anything with required options. `get_product` says so and gives the agent the link instead.
Configurable products go in by variant SKU, and grouped ones by their members' SKUs.

## The handoff

An agent's cart is a cart like any other, marked as built by an agent. Its id lives in the agent's
conversation, on someone else's servers, so it's never the cart the shopper checks out with.
`start_checkout` closes it and returns a link to `/handoff/<token>`. Opening the link copies the
lines into the shopper's own cart: the signed-in customer's, else the guest cart their browser already
has, else a new one. Then it lands them on the cart to look it over.

- A closed agent cart can't be read or changed again, by the agent or anyone else.
- The link lasts 7 days and can be opened more than once. Opening it again into the same cart adds
  nothing, so a reload or a chat app previewing the link can't double the order.
- Prices are whatever the cart says at checkout, not what the agent saw.

## Turning it off

`sales/agents/mcp_enabled` turns it off for a channel (see [settings.md](settings.md)), and
`MCP_FRONT_ENABLED=false` takes the endpoint away entirely.

## Limits and logs

Once `RATE_LIMIT_ENABLED` is on, each client address gets `MCP_FRONT_RATE_LIMIT_PER_MIN` tool calls a
minute (60 by default), and 4 at once whatever that says. Every call is logged as `mcp: tool` with the
tool, the channel, the client address and whether it failed.

Reviews come back quoted, and the agent is told they were written by other shoppers. A review can
still try to talk an agent into something, which is one more reason the agent never pays: whatever it
put in the cart, the shopper sees it before buying.
