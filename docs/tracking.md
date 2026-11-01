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

| Kind | Platform | Needs |
|---|---|---|
| `ga4` | Google Analytics 4, Measurement Protocol | analytics |
| `meta_capi` | Meta Conversions API | ads |
| `bing_uet` | Microsoft Advertising, UET Conversions API | ads |
| `openai` | OpenAI Ads Conversions API | ads |
| `cj` | CJ Affiliate, server-to-server postback | ads, or none with `general/privacy/cj_cookie_necessary` |
| `awin` | Awin Conversion API | ads, or none with `general/privacy/awin_cookie_necessary` |
| `klaviyo` | Klaviyo Events API | personalization, plus the email's marketing opt-in |
| `attentive` | Attentive eCommerce events | personalization, plus the email's marketing opt-in |
| `webhook` | Your own endpoint, signed JSON | analytics |

Until the dashboard pages land, set them up from the command line:

```bash
skelet cli tracking kinds                                  # what this build can send to, and each one's settings
printf '%s' "$GA4_API_SECRET" | skelet cli tracking add --secret-stdin --channel <id> --kind ga4 --name "GA4" \
  --set measurement_id=G-XXXXXXXXXX --set validate=true
skelet cli tracking test <destination-id>                  # one sample purchase, sent now, with the platform's answer
skelet cli tracking deliveries <destination-id>            # what was sent, retried or given up on
```

Catalogue accounts use the same destination storage and delivery history, with a separate
worker for large uploads. Their kinds are `catalog_openai` and `catalog_webhook`; shopper
events never go to them. See [catalogue delivery](catalogue-delivery.md) for the shared
system, account setup and HTTP/SFTP configuration.

## GA4 and Google Tag Manager

**The recommended setup loads no script.** Add a `ga4` destination. Page views, product views, checkout steps,
cart changes and orders reach GA4 through the Measurement Protocol. Each event carries the visitor's id as GA's
`client_id`, their analytics session as `session_id`, and their consent as Consent Mode's `ad_user_data` and
`ad_personalization`. The session is a first-party cookie, set only with analytics consent.

**Server-side GTM:** set the `ga4` destination's endpoint to your server-side GTM container's URL. The container's GA4
client receives the same Measurement Protocol requests and fans them out to whatever tags you run there.

**A web GTM container** is optional: set `general/analytics/gtm_container_id`. The storefront then loads it:

- only after the visitor allows a cookie category, with Consent Mode v2 set to denied by default and updated from
  their choice
- after the page has loaded and gone idle
- from your own domain if `general/analytics/gtm_script_origin` names your server-side container

Shop events are pushed to its `dataLayer` in GA4's ecommerce shape with an `event_id`, behind an `ecommerce: null`
reset.

> **Don't count twice.** GA4 doesn't dedupe between the Measurement Protocol and a browser tag. Use either the `ga4`
> destination or a GA4 tag in your web container, not both. The ad platforms (Meta, Microsoft, OpenAI) do dedupe on
> the event id, so their browser pixels and destinations can run side by side.
