# Concepts and vocabulary

The words Skelet uses for its own moving parts. If a term in the code or the
dashboard didn't mean what you assumed, it's probably on this page.

Read this before the other docs. Everything else assumes these words.

## Channel

**A channel is one market.** A country, a brand, or a language edition -
whatever you'd call "a shop" if you were running several from one back office.
It owns the commerce policy for that shop: its currency, the locales it serves,
its public host, whether prices are quoted tax-inclusive.

This is the single most important word in the project. Almost every list, form
and save in the admin is scoped by it.

The catalog underneath is shared. One product row can be sold on every channel,
at a different price on each, hidden on some. Customers and orders are *not*
shared - each belongs to the channel it was created on.

Two facts do most of the work:

- **A channel has exactly one currency.** Selling in two currencies means two
  channels.
- **Translations are not keyed by channel.** Content is keyed by *locale only*.
  Two channels serving `de` show the same German text.

Full behaviour: [channels.md](channels.md).

> **The word is "channel" everywhere.** The `channels` table, the `X-Channel-Id`
> header, `channelId` throughout the GraphQL schema, `effectiveChannelId` in the
> dashboard, and every label in the admin UI.

## Scope

**Scope is the answer to "which channel am I editing right now?"** In the
dashboard it's the header dropdown, and it has two modes:

- **Global** - you're editing the install-wide value. A setting saved here
  becomes the default for every channel.
- **Channel** - you're editing one channel. The same setting saved here
  overrides the global value for that channel only.

Scope decides what lists show you and what your saves write to. There's exactly
one source of truth for it, `useConfigScope()` in
`apps/dashboard/src/lib/config-scope.ts` - no per-page overrides, no hidden
defaults.

The admin and the storefront resolve scope by *different* rules on purpose. On
the storefront, no channel header means fall back to the default channel. In the
admin, **no header means all channels, not the default one.** Treat them as one
rule and a global-scope admin silently sees only the default store.

### The scope ladder

Settings resolve over three levels, most specific winning:

```
default  (no channel, no locale)   ← the base value
  └─ channel                       ← this channel overrides the base
       └─ channel + locale         ← this channel, in this language
```

Not every setting is overridable. Each declares itself **global** (settable only
at the base level) or **channel** (overridable). If the dashboard won't let you
change a setting while a channel is selected, it's global-scoped - switch the
header to Global, and know that it changes everywhere.

## Listing

**A listing is the junction row that puts a global thing on a channel.** It's
the pattern that makes one catalog serve many shops.

Products, categories, CMS pages, blog posts, menus and inventory locations are
all global - one row, shared. A listing row for (entity, channel) decides
whether that entity appears on that channel and how. **No row means hidden.**

Product listings are the fullest example: a listing carries `is_published`,
`price`, `compare_at_price`, `special_price` with its own window, and `url`. So
one product can be live in the US at one price, unpublished in the EU, and on
sale in the UK, with a single catalog row underneath.

The tables are `product_channel_listings`, `category_channel_listings`,
`cms_page_channel_listings`, `blog_post_channel_listings`. The Go types are
`ProductChannelListing`, `CategoryChannelListing`, and so on.

## Locale and currency

**Locale** is the content language, a single BCP-47 tag from the `Locale`
header. On the storefront it's clamped to the channel's supported set, so asking
for a locale the channel doesn't serve snaps to that channel's default rather
than returning blanks. In the admin it is deliberately *not* clamped, because
translation tooling has to reach locales the channel doesn't currently serve.

**Currency** is per channel, one each, and it's what the cart charges and the
order settles in. A storefront can *display* other currencies via
`presentmentCurrencies`, but that's presentment only - checkout still charges
the channel's own currency.

A language edition is usually **not** a reason for a second channel. Add a
locale to the existing one. Reach for a second channel when the commerce policy
differs - currency, tax, pricing - not when only the words do.

## Signals, and its four surfaces

**Signals** is the built-in analytics stack. It's a separate Go module
(`apps/backend/signals`) and a separate process (`cmd/signals` there) writing to a
separate database (`signals-db`), never the commerce one. It answers over one surface: the gRPC contract in
`proto/skelet/signals/v1/`, which only core calls. The dashboard talks to core,
and core relays.

Its four dashboard surfaces have their own names, and they are just names -
there's no shared subsystem behind them beyond the event spine:

| Name | Route | What it shows |
|---|---|---|
| **Lens** | `/lens` | Web analytics - pages, sessions, site search, web vitals, JS errors, session recordings, bot filtering |
| **Synapse** | `/synapse` | Customer analytics - profiles, segments, funnels, retention, lifecycle flows |
| **Pulse** | `/pulse` | Email and marketing engagement - opens, clicks, attributed revenue, top links |
| **Prism** | `/prism` | Storefront A/B experiments - assignment, variants, results |

### The spine

**The spine** is the append-only event log everything above reads from. It
lives in ClickHouse, which comes with the suite. `cmd/ingest` is the public
beacon endpoint that writes into it, and an optional NATS JetStream queue can
sit between the two for durability under spikes.

Core writes to it too - a placed order, a cart change, an email opened - over
the contract's `IngestService`. Those calls carry a **system token**: core
writing as itself rather than for an admin, since the event bus that raises
them has no caller. It authorizes the writes and nothing else, and an admin
token does not authorize them.

## Delivery and extension points

**Event bus** - in-process, not durable. `-ing` events fire pre-commit and can
abort or enrich; `-ed` events fire post-commit and are fire-and-forget. Good for
wiring domains together inside one process.

**Outbox** - the durable half. A Postgres transactional outbox relays async work
that has to survive a restart. When something must not be lost, it goes here,
not on the bus.

**Webhooks** - outbound HTTP to *observers*. Sync (pre-commit, can veto) or
async (queued, retried on a backoff ladder), HMAC signed.

**Remote providers** - outbound HTTP to *implementers*. Same shape as a webhook,
opposite intent: instead of observing an event, the endpoint fulfils a role the
platform needs, like quoting tax or rating a shipment. See
[remote-providers.md](remote-providers.md).

**Plugins** - Go packages compiled into the binary, written against the public
SDK in `apps/backend/sdk`. Payment gateways are plugins, on purpose: PCI scope
argues for in-process code over an HTTP hop. `plugins/example` exercises every
registration verb.

## Names you'll meet

Third-party pieces, and one of ours, that read as jargon until you've seen them
once:

| Name | What it is |
|---|---|
| `vetrina-demo` | The example storefront that ships in-tree. A template to fork, not a dependency - deselect it and nothing breaks |
| Valkey | Redis-compatible store. Refresh tokens and cache |
| Garage | Self-hosted S3-compatible object storage. Media, in dev and on-prem |
| OpenSearch | Search index. Optional - the app runs without it |
| ClickHouse | Columnar DB. The Signals spine, and log retention under the `logs` profile |
| NATS JetStream | Durable queue. Optional, in front of beacon ingest |
| Mailpit | Dev SMTP catcher. Nothing leaves your machine |
| Vector | Log shipper, container stdout into ClickHouse |
| gqlgen | Schema-first GraphQL codegen for Go |
| sqlc | Generates Go from SQL. There are no hand-written DB calls |
| air | Backend hot reload in dev |

## Where to go next

| You want | Read |
|---|---|
| How channels, locale and currency actually behave | [channels.md](channels.md) |
| Where the code lives and how a domain package is shaped | [architecture.md](architecture.md) |
| The commands you'll run daily | [workflow.md](workflow.md) |
| Per-domain engineering notes and footguns | [commerce-model.md](commerce-model.md) |
| Who can do what, at which scope | [permissions-enforcement.md](permissions-enforcement.md) |
