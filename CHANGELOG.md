# Changelog

Notable changes per release. Format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/);
versions follow [Semantic Versioning](https://semver.org/), with the `0.x` caveat
that breaking changes ride the MINOR until `1.0.0`.

The release process, and which version number means what, is in
[docs/releasing.md](docs/releasing.md).

## [Unreleased]

## [0.1.0] - 2026-08-30

First public release.

**This is a 0.x release and is not ready to run a live store on.** The feature
set below is implemented and covered by tests, including an end-to-end suite over
the money paths, but it has not been run in production by anyone. Expect rough
edges, gaps you only find under real traffic, and breaking changes: until `1.0.0`
there is no compatibility promise, and a breaking change rides the MINOR. Treat it
as something to evaluate, build against and report problems from, rather than
something to take payments with.

Two things worth knowing before you try it. Everything runs in containers and a
fresh clone comes up as a working shop, so evaluating it is cheap. And the SDK,
the remote-provider contract and the product version move independently, so a
plugin you write against `0.1.0` states its own compatibility range rather than
tracking the platform.

### Added

- Backend: catalog, cart, checkout, orders, payments, inventory, promotions,
  subscriptions, B2B, loyalty and referrals over a Go + PostgreSQL core, with two
  separate GraphQL surfaces for the admin and the storefront.
- Multi-channel and multi-locale throughout: one catalog serving several
  storefronts in different currencies and languages, with per-channel listings,
  scoped permissions and a scoped settings registry.
- Admin dashboard (React) and a reference storefront (Next.js).
- Tier 1 plugin system with an SDK, and remote providers over a versioned wire
  contract.
- Six payment gateways, each in its own repository.
- Signals: an append-only analytics spine, web analytics, session recording and
  customer profiles, on its own database and pluggable between PostgreSQL and
  ClickHouse.
- Search behind a pluggable engine, with a PostgreSQL tier and OpenSearch.
- Operations: a transactional outbox, HMAC-signed webhooks, a scheduled-jobs
  engine with retries and alerting, encrypted config secrets, and nightly
  encrypted database backups.

[Unreleased]: https://github.com/skeletg/skelet/compare/v0.1.0...develop
[0.1.0]: https://github.com/skeletg/skelet/releases/tag/v0.1.0
