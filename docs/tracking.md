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
| `google_ads` | `tracking-google-ads` | Google Ads conversions, Data Manager API | ads |
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

## Signing out

**Signing out starts a new visitor.** The next request after a sign-out gets a new visitor id and analytics session,
so on a shared computer the next person isn't joined to the account that left.

## GA4 and Google Tag Manager

**The recommended setup loads no script.** Add a `ga4` destination. Page views, product views, checkout steps,
cart changes and orders reach GA4 through the Measurement Protocol. Each event carries the visitor's id as GA's
`client_id`, their analytics session as `session_id`, and their consent as Consent Mode's `ad_user_data` and
`ad_personalization`. The session is a first-party cookie, set only with analytics consent.

**Server-side GTM:** set the `ga4` destination's endpoint to your server-side GTM container's URL. The container's GA4
client receives the same Measurement Protocol requests and fans them out to whatever tags you run there.

**No Google script runs in the browser.** That keeps every page as fast as it can be, and means each event reaches
Google once. A web GTM container isn't offered: everything a browser tag would send already goes server to server,
and a GA4 tag beside the `ga4` destination would count every event twice, since GA4 doesn't dedupe between the two.

## Google Ads

**Conversions go server to server** through the `google_ads` destination, which sends each purchase to Google's Data
Manager API - Google's route for imported conversions. It carries:

- the ad click: `gclid`, `gbraid` or `wbraid`, which the storefront keeps for 90 days with advertising consent
- the order number, the order value with tax and shipping, and the currency
- the shopper's email and phone, normalised and SHA-256 hashed as Google specifies - enhanced conversions, which
  match an order whose click id is gone
- the visitor's consent, as `adUserData` and `adPersonalization`

A purchase with no click id, email or phone isn't sent: Google could match it to nobody.

Setting one up:

1. In Google Ads, create a conversion action whose source is **Website (Import from clicks)** and note its ID.
2. In a Google Cloud project, enable the **Data Manager API**. Create a service account and download its key, or use
   an OAuth client's refresh token (an `authorized_user` credentials file).
3. Give that account access to the Google Ads account, or to a manager account above it.
4. Add the destination, checking it first with `validate_only`, which records nothing:

```bash
skelet cli plugin enable tracking-google-ads             # compiles it in; restart the backend
TRACKING_SECRET="$(cat service-account.json)" skelet cli tracking add --channel <id> --kind google_ads \
  --name "Google Ads" --set customer_id=1234567890 --set conversion_action_id=<id> --set validate_only=true
skelet cli tracking test <destination-id>
```

Set `login_customer_id` when the credentials reach the account through a manager account. Remove `validate_only` once
the test is accepted.

**Remarketing is not browser-based.** Remarketing to anonymous visitors - showing ads to someone who viewed a product
and left - needs Google's browser tag and its cookies, which the shop doesn't load. The planned route is **Customer
Match**: uploading the hashed emails of customers who opted into marketing, so Google can reach them on Search,
YouTube, Gmail and Shopping, exclude them from new-customer campaigns, and use them as a signal for Performance Max. It
reaches known customers only, and needs an Ads account with a good policy and payment history. View-through
conversions and Consent Mode's modelled conversions also depend on a browser tag, so they aren't available.
