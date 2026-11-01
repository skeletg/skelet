# Documentation

One file per question. **Start with [concepts.md](concepts.md)** - it defines the
vocabulary the rest assumes.

A new doc gets a row here rather than a link buried in a section, or nobody finds it.

## Working on the code

| Doc | What's in it |
|---|---|
| [concepts.md](concepts.md) | The vocabulary: channel, scope, listing, locale, the Signals suites. Read first |
| [layout.md](layout.md) | What every directory at the root is for, dotfiles included |
| [architecture.md](architecture.md) | Where the code lives, the GraphQL surfaces and the Signals wire contract, the shape every domain package follows |
| [workflow.md](workflow.md) | Daily commands, the Docker setup, what to regenerate after a schema change, which apps run |
| [conventions.md](conventions.md) | House rules per app - the dashboard's staged-save convention most of all |
| [commerce-model.md](commerce-model.md) | Per-domain engineering notes and footguns |
| [scripts.md](scripts.md) | Every command, what it does and when to reach for it |
| [plugins.md](plugins.md) | Extending the backend: Go plugins compiled in, or remote providers over HTTP |
| [env.md](env.md) | Every environment variable, which file it belongs in, and the tradeoffs behind the optional ones |
| [database.md](database.md) | The three migration streams, what the generated baselines are, and how to add to or regenerate one |
| [permissions-enforcement.md](permissions-enforcement.md) | The permission model - the contract for resolver authors |
| [releasing.md](releasing.md) | The branch model, what each version number means, and how a release is cut |

## Running a store

| Doc | What's in it |
|---|---|
| [checkout.md](checkout.md) | Delivery options at checkout - ship to address, store pickup, PO boxes - and how product types limit and price them |
| [store-availability.md](store-availability.md) | Public stores, per-store product stock, and development demo fixtures |
| [cart-offers.md](cart-offers.md) | Carts an admin builds and sends as a link - offer prices, custom lines, locks, expiry and the recipient's checkout |
| [channels.md](channels.md) | What's global vs per-channel, the scope ladder, how locale and currency resolve |
| [deploying.md](deploying.md) | The two deployment shapes, putting an edge in front, health, backups, logs |
| [permissions.md](permissions.md) | Configuring roles, plus the generated slug reference |
| [import-export.md](import-export.md) | Bulk data in and out: the resource registry, the job model, and how to add a resource or a format |
| [settings.md](settings.md) | Every configurable knob, its scope and default |
| [tracking.md](tracking.md) | Server-side tracking: how events reach GA4, GTM, ad, affiliate and marketing platforms, and setting up a destination |
| [catalogue-delivery.md](catalogue-delivery.md) | Shared catalogue destinations: account setup, automatic uploads, signed webhooks, retries and provider plugins |
| [openai-product-feed.md](openai-product-feed.md) | OpenAI catalogue adapter: SFTP setup, eligibility and snapshot downloads |
| [privacy.md](privacy.md) | Cookie consent, what each order and destination records under which choice, counting visitors who decline analytics, and privacy-notice text |
| [webhooks.md](webhooks.md) | Subscribing to events: async vs sync, signing, retries, the event catalogue |
| [remote-providers.md](remote-providers.md) | Writing a remote provider: the signed envelope, limits, per-kind schemas |
| [backups.md](backups.md) | What needs backing up, and the tools to do it with |
