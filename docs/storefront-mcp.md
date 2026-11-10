# Shoppers' agents over MCP

Every storefront answers [MCP](https://modelcontextprotocol.io) at `/api/mcp`, so a shopper can ask
their own AI assistant to find something on your store and fill a cart with it. The agent searches,
compares, checks delivery and builds the cart. It never places the order: when the shopper is ready
it hands them a link, and they finish on the store like anyone else.

This is a different endpoint from the one in [mcp.md](mcp.md). That one reads your sales and orders
for your own team, behind a token. This one is public, anonymous, and sees what any visitor sees.

## Connecting

The address is the storefront's own: `https://shop.example.com/api/mcp`. **Agentic Commerce, MCP** in the
dashboard shows it for each store, with whether agents can use it right now and why not. There's no sign-in, so a
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
| `search_products` | Search and filter the catalog, with facets the agent can narrow by - and, with [embeddings](embeddings.md) on, products close in meaning to the words |
| `find_similar_products` | Products like one to five others, by id, SKU, link or exact name - the nearest by meaning that share a value with them on every attribute marked **Must match for similar products** ([embeddings.md](embeddings.md#similar-products)) |
| `get_product` | One product by id, SKU or link: price, stock, details, variants, reviews, its promotions, and which SKU goes in the cart |
| `get_deals` | The automatic promotions running now with their terms, and the products selling below their usual price, biggest saving first |
| `list_categories` | The category tree, a level at a time |
| `search_store_pages` | Your CMS pages, for shipping, returns, sizing and the like - with embeddings on, the passages that answer the question |
| `estimate_shipping` | Delivery options and prices to a country, for a cart if it has one |
| `get_cart` | What's in the cart it built |
| `update_cart` | Start a cart, or set SKUs in it to exact quantities, and apply a code |
| `start_checkout` | Close the cart and get the link for the shopper |

Every read goes through the storefront's own GraphQL resolvers, in-process, so prices, stock, private
attributes, search merchandising and promotions come out exactly as the storefront shows them. Cart
edits go to the cart service directly, with the same checks the storefront's own cart calls make.
`search_store_pages` keeps each channel's page text for a minute, so a page edit can take that long to
show up there by its words - and as long as the embedding job's next run to show up by meaning.

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

## The assistant on the storefront

The same tools also power a chat on the storefront itself. Turn on `sales/agents/assistant_enabled`
for a channel and Vetrina shows a sparkles button in its header that opens a full-height panel on the
right, docked the way the dashboard's chat is, or a drawer from the bottom on a phone. It answers through the store's own AI provider (Settings, AI), so it's
off by default: every message a shopper sends is a paid model call. The button only shows when the switch is on
and a provider that calls tools is set up - the storefront asks `assistantAvailable` with its live public config,
so taking the provider away hides the button within a minute rather than leaving a chat that can only say it's
unavailable.

- The conversation lives in the shopper's browser tab, and the cart tools work on the shopper's own
  cart: a signed-in customer's, or the guest cart in their cookie. A guest with no cart gets one the
  first time something goes in, and the storefront keeps it. There's no agent cart and no handoff here.
- Products come as cards under the reply: the assistant picks them with a `show_products` tool that
  only the storefront chat has, and Vetrina renders its own product cards, with the picture, price and
  add to cart button. An id that isn't a product on the store never becomes a card.
- A reply about one product gets a panel with its picture, price, stock, rating and public specs, through
  `show_product_details`, with the assistant's take laid out in it - a one-line summary, a few highlights,
  who it's good for and what to keep in mind - so the message itself stays a sentence. A reply that looked at one product and showed nothing else gets it anyway.
- `show_results` and `show_page` put buttons under a reply: to a search or a category or collection
  narrowed with facet filters and a sort, with how many products it shows, or to a category, collection or
  information page. The backend checks a listing has products and a page exists, and the storefront builds
  the address with its own filter grammar. A price range isn't part of that grammar, so it can't be sent.
- Replies end with two or three things the shopper might say next, offered as one-tap suggestions. They
  ride in the same answer from the model, so they cost no extra call. An empty chat opens with suggestions
  for the page the shopper is on - a product, a list, a search or an information page - read by
  `assistantPage` without asking the model anything.
- Each reply comes with a short recap of what it showed - the products it explained or carded, with
  their ids, and its buttons. The storefront keeps it with the conversation and sends it back, so a later
  "compare it with this one" knows which product "it" was.
- The storefront sends the page the shopper is on, filters and search words included, and
  `get_current_page` reads it back: a product's details, a product list with its filters, sort, products
  and the facets it can be narrowed by, or an information page's text. On a product list,
  `update_listing` changes the filters and sort, and the storefront goes to the new address straight away.
  It refuses a sort the store doesn't offer and a change that would empty the list.
- `get_deals` lists only automatic promotions with a public label that the shopper could get: one aimed
  at another store, outside its schedule, or kept for a customer group or segment they aren't in stays
  out, while a condition only a cart settles, like a minimum spend, is left to the terms. A code-only
  promotion stays out too, since its code is the shopper's to bring. The sale products it lists are
  ranked once a minute per store.
- What the assistant changes shows up at once: the reply carries the fresh cart, so the header count
  updates, and a Review cart button under the message opens the cart drawer to check out from. A line
  with choices made on its product page, like an engraving or a bundle's picks, isn't the assistant's to
  change - it says so, and the shopper changes it in the cart.
- A signed-in shopper can ask about their own orders on the store: `get_my_orders` lists the latest ten,
  and `get_my_order` reads one by its number - items, status, payment, tracking, its returns and what can
  still be sent back - and puts a button to its page (`/account/orders/<id>`) under the reply. A guest is
  asked to sign in. These three tools are the assistant's alone - an agent over MCP is anonymous and has no
  orders to read.
- `start_return` asks the store to take items back, through the same `requestReturn` the account pages use,
  so the shipped-quantity and already-returned limits hold. It never sends on the first call: it checks the
  lines and answers with a summary for the assistant to put to the shopper, and the summary's key rides in
  that reply's recap. Only a later call with `confirmed`, after the shopper's next message brought that recap
  back, sends it - the same order, lines, quantities, reasons and resolution, or it's a new summary to confirm.
  A return the recaps show as sent is refused, so a later turn can't send it twice. The store reviews every
  return as usual; the assistant is told never to promise a refund.
- If the model fails or runs out of time after it already changed the cart, the reply still carries the
  cart, a guest's new one included, and the storefront says the answer was cut short.
- The storefront picks the cart from the session, never from the model. The model never sees a cart id,
  addresses or payment details, so it can't be talked into using someone else's cart.
- Each turn is capped: 30 messages and 20,000 characters of history, 2,000 for the shopper's newest
  message (older ones and the assistant's own replies are trimmed to fit, never refused), six rounds of
  tool calls with at most 8 calls a round and 24 a turn, and a minute in all. A shopper gets two turns at
  once and `sales/agents/assistant_per_minute` a minute (10 by default, 0 for no limit) - the signed-in
  customer, else the address, a whole IPv6 /64 counting as one - on whatever `RATE_LIMIT_ENABLED` says. The
  AI overviews above search results keep the same pace, counted apart. Each store also has
  `sales/agents/assistant_daily_turns` (500 by default, 0 for no cap): past it the assistant says it isn't
  available until the rolling day turns over.
- **Assistant voice**, under Agentic Commerce, sets a store's `sales/assistant/name`, `sales/assistant/tone`
  and `sales/assistant/instructions`. The name shows as the panel's title on the storefront and is what the
  assistant calls itself; the tone is one line of its prompt; the instructions, up to 4,000 characters, go
  after its own rules, fenced off, with what they say about the store taken as fact and no say over what the
  tools, the cart or checkout do. They're part of the cached prompt, so a store pays for them once per turn
  rather than once per round.
- The system prompt and tools stay the same on every page - the page the shopper is on rides on their
  newest message - and the request marks them for prompt caching, so the six rounds of a turn don't pay
  for them six times.
- Replies are rendered as plain text with light formatting. A link only becomes clickable when it resolves
  to the store's own origin, so an address a review slipped into an answer stays text.
- While the assistant works, the panel says what it's doing - searching for the shopper's words, looking
  at a named product, checking delivery to a country, updating the cart - instead of a plain "Thinking".
  The backend streams a line as each tool call starts, from `/api/front/assistant`: it takes what
  `askShopAssistant` takes as a JSON body, sent as `application/json` (anything else is refused, so another
  site can't post to it from its visitors' browsers), and answers in newline-delimited JSON: `{"step": {"tool",
  "detail"}}` lines, a `{"cart": {"id"}}` line the moment the turn makes a guest a cart, and then `{"reply": ...}`,
  shaped like the mutation's answer, or `{"error": {"code"}}` with the codes above. The mutation is still there
  for a storefront that doesn't want the steps.
- Vetrina talks to it through a route handler at `/api/assistant`, not a server action - Next runs a
  page's server actions one at a time, so a slow answer would have held up add to cart. The route only
  takes same-origin JSON, and streams the steps on to the page before the answer.
- A streamed response has sent its headers before a guest's first cart exists, so the cart cookie can't
  ride on it. The route passes the cart line on, and the page hands the cart straight back with a `PUT` to
  the same route, which sets the cookie once the store confirms the cart - so a shopper who closes the chat
  mid-answer still keeps what it put in. If they made a cart of their own while the turn ran, theirs stays:
  once the answer is in, the turn's lines and codes are added to it instead. A `PUT` that doesn't get through
  is tried once more, and the header goes on showing the cart the cookie really holds.

### When the model fails

Every AI call - the assistant, the search overview, admin chat, SEO text and the rest - goes through the same
steps when the provider fails in a way that isn't the request's fault (overload, rate limits, the provider's own
errors, a dropped connection):

1. It tries the same model once more after a short pause - the provider's `Retry-After` when it gives one, up to a
   second.
2. Then it asks the fallback model under **Settings, AI, LLM provider** (`ai/llm/fallback_model`), from the same
   provider. Short writing on the quick model falls back to the main model when no fallback is set.
3. A model that fails three calls in a row rests for 30 seconds, its calls going straight to the fallback, so
   shoppers don't each wait on a provider that's down. The first call after the rest tries it again.

A request the provider turns down - a bad request, a refusal - is never retried elsewhere, and nothing is started
that can't finish inside the turn's own time limit. When both models fail, the shopper reads that the assistant
isn't available and that the search still finds everything; the overview simply isn't shown. Langfuse records which
model answered each call, the log says `ai: a model call failed, answering on the fallback`, and the metric
`ai.model.failovers` counts them by model.

## Measuring it

Every message the assistant answers is a paid model call, so the store keeps count:

- Each turn adds to a daily tally per store: turns, the ones that failed, the ones it couldn't answer, the ones
  that changed the cart or sent the shopper to it, and the tokens the provider billed, cached reads and writes
  apart. Couldn't answer is the same test the conversations' filter applies - nothing said and shown, cut short,
  every search empty, or the assistant saying the store's information didn't cover it - but counted whether or
  not conversations are kept. A failed turn
  still counts what it spent. A turn refused before the model ran - too fast, busy, past the daily cap -
  counts nothing.
- A cart the assistant changes is marked as assisted, and so is a cart a handoff link lands in, as the
  agent's. The first one to help keeps the credit while it's fresh, and helping again keeps it fresh. A guest's
  mark follows their cart when they sign in, and an order placed within 30 days of the last help carries it as
  `assisted_by`. After that the cart is just a cart, so one left for weeks and filled again by hand isn't the
  helper's sale.
- **Agentic Commerce, Overview** shows the numbers for the store picked in the header over the last 7, 30 or 90
  days, with messages answered per day, and lists what's switched on for AI shopping there - the AI provider, the
  assistant, its voice, kept conversations, shoppers' agents, search by meaning with how much of the catalog is
  embedded, and the cost prices - each saying what it still needs and opening where it's set. While conversations
  are kept, the couldn't-answer count opens them filtered to those.
- **Shopping assistant** on the dashboard Home (off until you add it) shows the last 30 days for the
  store picked in the header: messages answered and the share it couldn't answer, carts filled, assisted orders
  with their revenue and share of all orders, orders from shoppers' own agents, and the model cost - the tokens
  alone until it has prices. The cost needs your provider's
  prices under **Settings, AI, Usage pricing** - input and output per million tokens. Cache reads and writes
  left blank are priced like input tokens - enter your provider's cache prices for a closer estimate. A price that
  isn't a number is logged once and leaves the estimate off. `assistantReport` in the admin API answers the
  same, under `reports.view`.
- Signals gets an `assistant_turn` event per turn, on the shopper's profile - the customer, or the visitor
  for a guest - with what it did (the products it showed, cart changed, checkout, cut short) and never the words.
  `order_placed` carries `assisted_by` too, and an order the assistant or an agent helped also raises its own
  `assisted_order`, so a funnel from `assistant_turn` to `order_placed` works like any other, and the segment builder
  always offers *Chatted with the shopping assistant* and *Ordered with AI help*. Carts and orders name the products
  they hold, variants with the product they're sold through, so Signals can tell what came of what the assistant showed.

## Reading conversations

Turn on `sales/agents/assistant_transcripts` for a store and the assistant's conversations are kept, so you
can read what shoppers asked, what it answered, and how it got there. Off by default.

- Each turn keeps the shopper's message, the reply, what it showed, the suggestions it offered, the page they
  were on, the model, the tokens it used, and every round of the model's work: a summary of its thinking, any
  words it wrote along the way, and each tool it called with the arguments it sent and what the store answered.
- Only the model's last message reaches the shopper, and its prompt says so. When that last message has no
  words, the words it wrote beside a tool call after seeing a result stand in, as long as whatever they
  introduced was actually shown. The trace marks which words were sent, and the calls the model asked for in
  the last step a turn allows, which never ran.
- The turn reaches Signals through the event outbox, so one sent while Signals is down goes once it's back. A
  transcript past Signals' limits loses the longest tool outputs first, never the question or the answer.
- The thinking is a summary the model writes of its own reasoning, asked for only while transcripts are
  on and billed the same either way. Claude's 5-series models give one; other models, and providers behind
  an OpenAI-compatible endpoint that don't return reasoning, show the tool calls alone. No model hands over
  its raw reasoning.
- Conversations live in Signals, so nothing is kept while Signals is off. **Agentic Commerce, Conversations**
  in the dashboard lists them per store, newest first, with a search over messages and replies; opening one shows
  each turn with a **How it got there** section, read when you open it. A message the shopper got no answer
  to, neither words nor anything shown, is flagged there and counted under **Needs a look**. A very long
  conversation opens on its latest 200 messages. Reading them takes the `assistant.view` permission.
- The list filters to the conversations with a message the store couldn't answer - any of them, or one kind:
  nothing said and nothing shown, cut short, every search of the catalog and pages came back empty, or the
  assistant said the store's information didn't cover it. That last one is the model's own call - its prompt asks it
  to mark such a reply, and the mark is taken off before the shopper sees it. Each filter says how many
  conversations it holds, every row says how many of its messages went unanswered, and filtered, a row shows the
  latest of them and opens its conversation at that message. `?unanswered=any` (or `no_answer`, `cut_short`,
  `found_nothing`, `unsure`) opens the list already filtered. It's the list of what to add to the catalog and the
  store's pages.
- A turn is only kept for a shopper who accepted analytics - signed in or not, since an account isn't consent:
  the storefront sends the chat id only for them, and without one nothing is kept. A guest who signs in mid-chat
  keeps one conversation, now under their account.
- The storefront names the chat, and the backend files it under an id made from the store, the shopper
  and that name, so a chat id sent from one browser can't add to someone else's conversation.
- `signals/assistant/retention_days` (90 by default, 0 keeps them) drops each message once it's that old, and
  a conversation with its last one - and never later than `signals/analytics/spine_retention_days`, since they're
  kept on the same consent as the event log. They're deleted, never archived, whatever the retention action. Erasing a
  customer drops their conversations and their messages in anyone else's, including the ones they had as a guest
  before signing in - checked again once the erasure sweep runs, for sign-ins that hadn't landed yet.

## What Signals makes of it

With Signals on, the AI's work also shows up as analytics. None of it holds a shopper's words, and none of it is
needed for the AI to answer, so with Signals off it's simply not there.

- **Agentic Commerce, AI insights** (`assistant.view`, `aiInsights` in the admin API) looks at the last 7, 30 or 90
  days:
  - **Chatted against everyone else.** People seen on the storefront (a page view or a chat) on two sides: the
    ones who talked to the assistant and the ones who didn't, each with how many bought, their orders, revenue in
    the store's currency and how many orders a return was asked for on. A chatter's orders count only from their
    first chat on, so what they'd have bought anyway before it isn't the assistant's.
  - **What it recommends.** The 20 products its turns showed most, with how many people saw each, and of them how
    many put it in their cart within 7 days and ordered it within 14. A variant counts for the product it belongs
    to.
  - **Shoppers' own agents.** Every MCP client by the name it gives itself: its calls, its searches and how many
    came back empty, and the tools it uses most. Below them, the orders placed from a cart an agent filled.
- **Lens, Search** adds two lists to the storefront's own searches: what shoppers asked the assistant or their
  own agents for that matched nothing in the catalog's words - search by meaning finds something close for almost
  anything, so that's where a gap shows - so the gaps you can't see in the search box show up too, and the AI
  overview per search - how often it was shown, a product in it followed, and the search carried into the chat.
  The searches listed are the assistant's and the agents' own words for the catalog, never the shopper's message.
- Tool calls and searches are counted per store and day, never per person, and an activity sent twice counts
  once. They age out with Lens's rollups (`signals/lens/rollup_retention_days`).
- `assistant_turn` carries the products it showed and `order_placed` the products ordered, so a segment of people
  who chatted, or ordered with the AI's help (the `assisted_order` event), works like any other - the segment
  builder offers both even before a store has any.
- A flow on **Abandoned cart the assistant filled** (`assisted_cart_abandoned`) takes the carts the assistant
  filled in place of the usual abandoned-cart flow. A store without one keeps sending its usual recovery mail for
  them.

## Turning it off

`sales/agents/mcp_enabled` turns it off for a channel (see [settings.md](settings.md)), and
`MCP_FRONT_ENABLED=false` takes the endpoint away entirely.

## Limits and logs

Hosted assistants call from a handful of shared addresses, so the budgets per client address are wide:
`sales/agents/mcp_per_minute` tool calls a minute per store (600 by default, 0 for no limit), on whatever
`RATE_LIMIT_ENABLED` says, and always `MCP_FRONT_MAX_IN_FLIGHT` calls at once (32). A whole IPv6 /64 counts as one address. The tools' searches by meaning are counted by this
budget rather than the narrower per-shopper one the storefront's own search has. A call gets 20 seconds. Every call is logged as
`mcp: tool` with the tool, the channel, the client address and whether it failed.

The client address comes from `X-Forwarded-For`, which the storefront passes on as it gets it. Keep an
edge like the bundled Caddy in front of the storefront, so what a client sends there gets replaced
rather than trusted.

Reviews come back quoted, and the agent is told they were written by other shoppers. A review can
still try to talk an agent into something, which is one more reason the agent never pays: whatever it
put in the cart, the shopper sees it before buying.
