# Server-side tracking: GA4, GTM, ad and affiliate platforms

How shop events reach analytics, ad, affiliate and marketing platforms. The default sends everything server to
server, so the storefront loads no tracking script. What each event needs from the visitor's cookie choices is in
[privacy.md](privacy.md).

## How events flow

| Event | Reported by |
|---|---|
| `page_view`, `view_item`, `begin_checkout` | The storefront, as a beacon to its own `/t/e`. The storefront server adds the visitor's consent, click ids, analytics session and browser, then forwards it to the backend |
| `add_to_cart`, `remove_from_cart` | The backend, when the cart changes |
| `purchase`, `refund` (also on cancellation) | The backend, from the order, using the consent given at purchase |
| `sign_up` | The backend, when a shopper creates an account |

Each event goes to every enabled destination on its channel that wants it, if the visitor allowed what that platform
needs. Delivery is queued and retried: after 10s, 1m, 5m, 30m and 2h, then dead. One failing event never holds up
the rest. A purchase's event id is the order id, so a platform that also gets the event from a browser tag counts it
once.

## Destinations

**Every destination is in development.** Each is built from its vendor's documentation and tested against recorded
requests, but none has yet run against a production account. They are off by default and compiled into no build
until enabled. Verify one against a real account before relying on it in production:

```bash
skelet cli plugin enable tracking-ga4                      # compiles it in; restart the backend
```

| Kind | Plugin | Platform | Needs |
|---|---|---|---|
| `ga4` | `tracking-ga4` | Google Analytics 4, Measurement Protocol | analytics |
| `mixpanel` | `tracking-mixpanel` | Mixpanel Import API | analytics |
| `meta_capi` | `tracking-meta` | Meta Conversions API | ads |
| `bing_uet` | `tracking-bing` | Microsoft Advertising, UET Conversions API | ads |
| `openai` | `tracking-openai` | OpenAI Ads Conversions API | ads |
| `cj` | `tracking-cj` | CJ Affiliate, server-to-server postback | ads, or none with `general/privacy/cj_cookie_necessary` |
| `awin` | `tracking-awin` | Awin Conversion API | ads, or none with `general/privacy/awin_cookie_necessary` |
| `klaviyo` | `tracking-klaviyo` | Klaviyo Events API | personalization, plus the email's marketing opt-in |
| `attentive` | `tracking-attentive` | Attentive eCommerce events | personalization, plus the email's marketing opt-in |
| `webhook` | `tracking-webhook` | Your own endpoint, signed JSON | analytics |

Until the dashboard pages land, set them up from the command line:

```bash
skelet cli tracking kinds                                  # what this build can send to, and each one's settings
TRACKING_SECRET=<api secret> skelet cli tracking add --channel <id> --kind ga4 --name "GA4" \
  --set measurement_id=G-XXXXXXXXXX --set validate=true
skelet cli tracking test <destination-id>                  # one sample purchase, sent now, with the platform's answer
skelet cli tracking deliveries <destination-id>            # what was sent, retried or given up on
```

## Mixpanel

**No Mixpanel script is needed.** The `mixpanel` destination sends events through Mixpanel's Import API with a
service account, to the host that matches the project's data residency (`region`: `us`, `eu` or `in`). Identity
follows Mixpanel's simplified ID merge:

- `$device_id` is the storefront's visitor id, which exists only with analytics consent.
- `$user_id` is the customer id once the shopper is signed in. An event carrying both joins the anonymous history
  to the customer, so Mixpanel works out `distinct_id` itself.
- An event with neither goes nowhere. Visitors who decline analytics are counted as nobody in Signals instead
  (see [privacy.md](privacy.md)); nothing about them reaches Mixpanel.
- `$insert_id` is derived from the event id, so a retry counts once. Email and phone are never sent.

**Session replay and heatmaps** need Mixpanel's browser library, and are off until you set
`general/analytics/mixpanel_replay_token` to the project token (with `mixpanel_region` and
`mixpanel_replay_percent`). The storefront then records a share of visits:

- only for visitors who allowed analytics. Before a choice, after a refusal, or after a withdrawal, nothing loads and
  nothing is sent. A withdrawal stops the recording, and the next request deletes Mixpanel's cookie
- with every typed input masked, and no events: no clicks, page views or `$opt_in`. Heatmaps come from the recordings
- from the bundle, not Mixpanel's CDN. The only third-party hosts are Mixpanel's ingestion hosts, in the CSP's `connect-src`
- joined to the customer on sign-in, the same `$user_id` the server sends. Anonymous recordings keep the library's own
  device id, because Mixpanel offers no supported way to give it ours

**Signing out starts a new visitor.** The next request after a sign-out gets a new visitor id and analytics session,
and replay resets its device, so on a shared computer the next person isn't joined to the account that left.

## GA4 and Google Tag Manager

**The recommended setup loads no script.** Add a `ga4` destination. Page views, product views, checkout steps,
cart changes and orders reach GA4 through the Measurement Protocol. Each event carries the visitor's id as GA's
`client_id`, their analytics session as `session_id`, and their consent as Consent Mode's `ad_user_data` and
`ad_personalization`. The session is a first-party cookie, set only with analytics consent.

**Server-side GTM:** set the `ga4` destination's endpoint to your server-side GTM container's URL. The container's GA4
client receives the same Measurement Protocol requests and fans them out to whatever tags you run there.

**A web GTM container** is optional: set `general/analytics/gtm_container_id`. The storefront then loads it:

- only after the visitor allows a cookie category, with Consent Mode v2 set to denied by default and updated from
  their choice. Withdrawing every category reloads the page without the container, since Google's tags still send
  cookieless pings under "denied"
- after the page has loaded and gone idle
- from your own domain if `general/analytics/gtm_script_origin` names your server-side container

Shop events are pushed to its `dataLayer` in GA4's ecommerce shape with an `event_id`, behind an `ecommerce: null`
reset.

> **Don't count twice.** GA4 doesn't dedupe between the Measurement Protocol and a browser tag. Use either the `ga4`
> destination or a GA4 tag in your web container, not both. The ad platforms (Meta, Microsoft, OpenAI) do dedupe on
> the event id, so their browser pixels and destinations can run side by side.
