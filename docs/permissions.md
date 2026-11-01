# Permissions

How to decide who in your organisation can do what, and how to configure it.

This is the operator's guide. If you are writing a resolver and need to know which guard
helper to call, you want
[`permissions-enforcement.md`](permissions-enforcement.md)
instead - that one covers the enforcement patterns and the static coverage test.

## The model

An admin gets permissions through **roles**. A role holds a list of **grants**, and each
grant is a triple:

```
(permission, channel?, locale?)
```

Both scope columns are optional, and leaving one empty means "any":

| Channel | Locale | The grant applies to |
|---|---|---|
| any | any | everything - a global grant |
| `EU` | any | the EU channel, in every locale |
| any | `de` | German content, on every channel |
| `EU` | `de` | only EU's German content |

Grants stack. An admin's roles are unioned and **the broadest grant wins** - if one role
gives `catalog.manage` on EU and another gives it globally, the global one applies. There
is no deny rule; you cannot subtract a permission by adding a narrower grant.

Two rules that surprise people:

- **`manage` implies `view`, never the reverse.** Every resource registers a
  `<resource>.view` / `<resource>.manage` pair, and `manage` is a strict superset - an
  admin with only `orders.manage` passes an `orders.view` check. So granting both is
  redundant; grant `manage` alone.
- **Superusers bypass everything.** The check short-circuits before roles are consulted, so
  a superuser's role assignments are decoration. Use the flag sparingly - it is not
  scoped, auditable per-resource, or revocable per-channel.

There is no `all_channels` toggle. "Global for X" is derived: it means at least one grant
row for X has both scope columns empty.

## Setting up the common roles

**Channel manager** - runs one storefront end to end, can't touch the others. Grant the
resource `manage` slugs with Channel set to that channel and Locale left as any. They will
see only their channel in the picker, only their channel's orders and customers, and get a
`FORBIDDEN` on anything belonging to another channel.

**Translator** - edits German copy everywhere. Grant the content `manage` slugs with Channel
left as any and Locale set to `de`. This is why locale scope exists independently of
channel scope.

**Support** - reads everything, changes nothing except what they must. Grant the `view`
slugs globally plus the specific `manage` slugs their job needs, e.g. `orders.refund`.

**Read-only auditor** - the `view` slugs globally, nothing else.

Roles are edited at `/admins/roles/:id` in the dashboard, under **Permission grants**. Add
one row per (permission, channel, locale) combination; add the same permission twice to
grant it on two different scopes.

## Why can't my admin see something?

Work down this list - it is roughly the order of likelihood.

1. **They have `view` but the thing is channel-scoped elsewhere.** List queries filter
   silently rather than erroring, so a scoped admin sees a short list, not a warning. A
   product detail page will also render with no listing rows for channels they can't see.
2. **They have the permission on the wrong scope.** A grant on `(EU, any)` does nothing for
   a US order. Check the grant rows, not just the permission name.
3. **The action touches a row shared across channels.** Product content, attribute values,
   media, downloads and subscription terms are one row rendered by every channel that lists
   the product, so editing them requires the permission on **every** listing channel (or a
   global grant). A single-channel admin is refused on a product listed in two channels -
   deliberately, since the edit would otherwise leak into a channel they can't see. If you
   need genuinely per-channel copy, give each channel its own locale.
4. **It is a global-only resource.** These need a global grant, and a channel-scoped grant
   never satisfies them:
   - install settings: currencies, countries, locales, the audit log, the scheduler, cache
     purges, the email suppression list, and the config-orphan actions;
   - access: admin users, roles and integration tokens (and nobody can hand out a grant wider
     than their own);
   - shared catalog and content: product types, attribute groups and definitions, collections,
     CMS schema, snippets, block types and tags, blog authors, categories and tags, inventory
     locations, and edits to shared media;
   - install-wide rows: webhooks, remote providers and gift cards with no channel, promotion
     rule ordering and simulation;
   - importing products or categories, and any data export or import that names no channel.

   Creating a new CMS page, blog post, category or menu only needs the permission somewhere,
   since the new row isn't listed on any channel yet. Adding it to another channel, or taking
   a listing away, needs the permission on every channel that already lists it, so one store
   can't claim another store's page and lock its admins out.
5. **They have zero grants for the slug.** Then they see an empty list and no buttons,
   which is correct.

## Integration tokens are not admins

API tokens for machine callers authenticate separately from admin roles. A token holds a
flat set of permission slugs and, optionally, **a channel pin**: an unpinned token's grants
hold globally, while a pinned token's grants only satisfy checks scoped to that one channel -
global gates refuse it and channel-filtered lists show only its channel. Pin the token for
anything single-storefront (a feed export, a per-store sync), grant the narrowest set of
slugs it needs, and issue one token per integration so you can revoke them independently.
There is no locale scoping on tokens.

A token can do whatever its slugs allow, the same as an admin holding them - moderate
reviews, adjust store credit, run imports and exports, edit the catalog. What it does is
recorded against the integration rather than a person: the audit log names it (filter the
log by integration to see everything one system did), and so do the records that keep an
author, like a moderated review, an answered question, a store-credit adjustment or a data
job. Inventory syncs that name no source are labelled `integration:<name>`.

A few things only a person can do, and a token gets a clear `ADMIN_ONLY` refusal there:
your own account, password, two-factor and sessions, your preferences and notification
inbox, AI chat, draft preview links, claiming a CMS page, acting as a customer, and managing
admins and other tokens.

## Permission reference

Slugs are registered in code - each domain package declares its own in `init()` - and this
table is generated from that registry by `skelet gen`, so it cannot drift from what core
declares.

The naming convention is `<resource>.view` and `<resource>.manage`, plus extras where an
action deserves its own gate (`orders.refund`, `reviews.moderate`).

**Grants** carries the registry description where one is written, and otherwise the display
name - which for most slugs just restates the slug. The descriptions exist for the grants
where the name undersells the power: `orders.refund` moves money, `admins.manage` can
escalate to anything else. Those are the rows worth reading before you hand them out.

**The roles UI can list slugs this table doesn't**, for two reasons, and the difference
matters when you are debugging a grant that seems to do nothing:

- **Enabled plugins register their own.** Those are runtime additions and belong to the
  plugin, so they aren't part of the core reference.
- **Removed permissions leave inert rows behind.** Sync never deletes, so a slug dropped
  from the code registry stays in the database and keeps appearing in the picker while
  gating nothing at all. `lens.manage` is the live example today - Lens is query-only, its
  `.manage` twin guarded no write path and was removed, but the row survives. Granting it
  has no effect. If a permission is in the picker but not below, that is what you are
  looking at.

<!-- BEGIN GENERATED PERMISSIONS -->
<!-- Generated by cmd/docsref via skelet gen. Edits here are overwritten. -->

76 permissions in 35 groups.

### Products

| Slug | Grants |
|---|---|
| `products.view` | View Products |
| `products.create` | Create Products |
| `products.update` | Update Products |
| `products.delete` | Delete Products |

### Orders

| Slug | Grants |
|---|---|
| `orders.view` | View Orders |
| `orders.update` | Update Orders |
| `orders.refund` | Issue refunds and credit memos against any order in scope, including goodwill credits with no return behind them. Money leaves on this permission - grant it separately from orders.update. |

### Customers

| Slug | Grants |
|---|---|
| `customers.view` | View Customers |
| `customers.manage` | Manage Customers |

### Admins

| Slug | Grants |
|---|---|
| `admins.view` | View Admins |
| `admins.manage` | Create admins, assign roles and edit permission grants. Effectively lets the holder grant themselves anything else, so treat it as the highest privilege short of superuser. |

### Configuration

| Slug | Grants |
|---|---|
| `config.view` | View Configuration |
| `config.manage` | Manage Channel Configuration |
| `config.manage_defaults` | Edit settings at the base scope, which changes them for every channel at once. Distinct from config.manage, which only reaches the selected channel's overrides. |
| `config.manage_secrets` | Write or clear secret-typed settings - gateway API keys, webhook signing secrets, SMTP credentials. Separate from config.manage because repointing where money settles should not ride the same grant as changing a date format. |

### Catalog

| Slug | Grants |
|---|---|
| `catalog.view` | View Catalog Configuration |
| `catalog.manage` | Manage Catalog Configuration |

### Search

| Slug | Grants |
|---|---|
| `search.view` | View Search Status |
| `search.manage` | Reindex Search |

### Developer Tools

| Slug | Grants |
|---|---|
| `developer.playground` | Open the GraphQL playground and run introspection against the admin schema. Exposes the full admin API shape, so it belongs with developers rather than operators. |
| `webhooks.view` | View Webhooks |
| `webhooks.manage` | Manage Webhooks |
| `remote_providers.view` | View Remote Providers |
| `remote_providers.manage` | Manage Remote Providers |

### Promotions

| Slug | Grants |
|---|---|
| `promotions.view` | View Promotions |
| `promotions.manage` | Manage Promotions |

### Email Campaigns

| Slug | Grants |
|---|---|
| `campaigns.view` | View Campaigns |
| `campaigns.manage` | Manage Campaigns |

### Automation Flows

| Slug | Grants |
|---|---|
| `flows.view` | View Flows |
| `flows.manage` | Manage Flows |

### Reports

| Slug | Grants |
|---|---|
| `reports.view` | View Reports |

### Email Library

| Slug | Grants |
|---|---|
| `emails.view` | View Email Library |
| `emails.manage` | Manage Email Library |

### Experiments

| Slug | Grants |
|---|---|
| `experiments.view` | View Experiments |
| `experiments.manage` | Manage Experiments |

### Notifications

| Slug | Grants |
|---|---|
| `notifications.view` | View Notifications |

### Web Analytics

| Slug | Grants |
|---|---|
| `lens.view` | View Web Analytics |

### Checkout

| Slug | Grants |
|---|---|
| `checkout.view` | View Checkout Settings |
| `checkout.manage` | Manage Checkout Settings |

### Channels

| Slug | Grants |
|---|---|
| `channels.manage` | Manage channels |
| `channels.view` | View channels |

### Locales

| Slug | Grants |
|---|---|
| `locales.manage` | Manage locales |
| `locales.view` | View locales |

### Currencies

| Slug | Grants |
|---|---|
| `currencies.manage` | Manage currencies |
| `currencies.view` | View currencies |

### Countries

| Slug | Grants |
|---|---|
| `countries.manage` | Manage countries |
| `countries.view` | View countries |

### Media

| Slug | Grants |
|---|---|
| `media.manage` | Manage media library |
| `media.view` | View media library |

### Inventory

| Slug | Grants |
|---|---|
| `inventory.manage` | Manage inventory |
| `inventory.view` | View inventory |

### Blog

| Slug | Grants |
|---|---|
| `blog.view` | View Blog |
| `blog.manage` | Manage Blog |

### CMS

| Slug | Grants |
|---|---|
| `cms.view` | View CMS pages |
| `cms.manage` | Manage CMS pages |
| `cms.types_manage` | Manage CMS page types and attributes |
| `cms.approve` | Approve CMS pages for publishing |

### Reviews

| Slug | Grants |
|---|---|
| `reviews.view` | View Reviews |
| `reviews.manage` | Manage Review Questions |
| `reviews.moderate` | Approve, reject and edit customer reviews before they reach the storefront. Separate from reviews.manage so a moderator can clear the queue without reaching review settings or questions. |

### Product Q&A

| Slug | Grants |
|---|---|
| `qna.view` | View Product Questions |
| `qna.moderate` | Answer / Reject Product Questions |

### Search Merchandising

| Slug | Grants |
|---|---|
| `searchmerch.manage` | Manage Synonyms / Pins / Redirects |

### Forms

| Slug | Grants |
|---|---|
| `webforms.view` | View Forms |
| `webforms.manage` | Manage Forms |
| `webforms.submissions_view` | View Form Submissions |

### Payments

| Slug | Grants |
|---|---|
| `payments.manage` | Manage payment methods |
| `payments.view` | View payment methods |

### URL rewrites

| Slug | Grants |
|---|---|
| `urlrewrite.manage` | Manage URL rewrites |
| `urlrewrite.view` | View URL rewrites |

### Import & export

| Slug | Grants |
|---|---|
| `data.view` | See the transfer hub and its job history - what can actually be run is gated by each resource's own permission |

### Cache

| Slug | Grants |
|---|---|
| `cache.view` | View cache settings & activity |
| `cache.manage` | Purge cache & edit cache settings |

### Scheduled jobs

| Slug | Grants |
|---|---|
| `scheduler.view` | View scheduled jobs |
| `scheduler.manage` | Manage scheduled jobs (schedule, enable, run now, webhook jobs) |

### Audit log

| Slug | Grants |
|---|---|
| `audit.view` | View the admin audit log |
<!-- END GENERATED PERMISSIONS -->
