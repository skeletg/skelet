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
| `google_ads_audience` | `tracking-google-ads` | Google Ads Customer Match list | ads and personalization, plus the account's marketing opt-in; removals need none |
| `meta_capi` | `tracking-meta` | Meta Conversions API | ads |
| `bing_uet` | `tracking-bing` | Microsoft Advertising, UET Conversions API | ads |
| `openai` | `tracking-openai` | OpenAI Ads Conversions API | ads |
| `cj` | `tracking-cj` | CJ Affiliate, server-to-server postback | ads, or none with `general/privacy/cj_cookie_necessary` |
| `awin` | `tracking-awin` | Awin Conversion API | ads, or none with `general/privacy/awin_cookie_necessary` |
| `klaviyo` | `tracking-klaviyo` | Klaviyo Events API | personalization, plus the email's marketing opt-in |
| `attentive` | `tracking-attentive` | Attentive eCommerce events | personalization, plus the email's marketing opt-in |
| `webhook` | `tracking-webhook` | Your own endpoint, signed JSON | analytics |

### Setting up in the dashboard

**Marketing → Tracking** lists every platform with where it stands on the selected channel: on, off, failing,
waiting for its secret, not set up, its plugin switched off on the channel, or not in this build - with the
`skelet cli plugin enable <slug>` command that adds it. Admins need `tracking.view` to see it and `tracking.manage`
to change it.

Setting a platform up is guided:

1. **What you need** - what it sends, which values to collect from the platform and links into its own UI. Each
   field says where to find its value.
2. **Consent** - the cookie categories every event needs.
3. **Settings and events** - saved switched off, in the platform's test mode where it has one (GA4's validate,
   Google Ads' and OpenAI's validate-only, Awin's test flag; Meta's test event code is pasted in).
4. **Send a test** - one sample purchase, with the platform's answer shown as it came back, credentials redacted.
5. **Turn it on** - leaves test mode and switches it on in one save.

A destination's page keeps its settings behind the one reviewed Save, with **Send test**, the on/off switch and
**Delete** acting at once, its last successful send, and a **delivery log** of what is retrying and what gave up, with Retry on
the failed ones. The log shows the event and the platform's answer, never the event's contents, which hold the
shopper's email, IP and click ids. Credentials are write-only: stored encrypted and never shown again.

**Settings → Privacy** holds the cookie consent mode, the anonymous counts switch and the affiliate exceptions, and
shows which destinations each consent category feeds.

### Importing from Signals

The older Signals event destinations (now **Marketing → Signals event export**) forward Signals' own events - email
campaigns, flows, transactional and system events. **Import from Signals** on the Tracking page copies their GA4, Meta
and webhook ones here after a preview:

- each arrives switched off and asks for its secret again - Signals never hands one over
- their event filters don't carry over; the platform's default shop events apply
- a webhook changes format: one event per POST, signed with `X-Webhook-Signature-256`, where Signals posted batches
  signed with `X-Destination-Signature` - update the receiver before turning it on
- the Signals destination keeps running until you switch it off there, so switch one of the two off once the new one
  works, or events go twice. Importing the same one again does nothing

### From the command line

The same, for scripts:

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

**Refunds and cancellations restate the conversion.** Each one sends the order's transaction id again with what the
order is still worth after every refund so far, and Google Ads replaces the conversion's value. The Data Manager API
has no retraction, so a fully refunded order's value goes to 0 but it still counts as one conversion.

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

**Remarketing goes through Customer Match**, not a browser tag - remarketing to anonymous visitors (someone who viewed
a product and left) needs Google's browser tag and cookies, which the shop doesn't load. The `google_ads_audience`
destination keeps a Customer Match list of your customers instead:

- **Who joins:** a customer with an account who opted into email marketing, when they buy or sign up while allowing
  advertising and personalization cookies. Guests never join: nothing would tell the list when they leave.
- **Who leaves:** anyone who turns email marketing off, or whose account is deleted or erased. Removal is sent whatever
  their cookie choice.
- **What is sent:** only the email and phone, normalised and SHA-256 hashed, with the consent behind it.
- **The list fills from new events.** Customers who opted in before the destination was set up join on their next
  purchase.

Setting it up:

1. In Google Ads, accept the Customer Match terms and create a customer list (Tools > Shared library > Audience
   manager > Your data segments > Customer list). Note its ID.
2. Use the same credentials as the conversions destination, and add it with `--kind google_ads_audience`,
   `--set user_list_id=<id>` and `--set terms_accepted=true`. Nothing is uploaded until `terms_accepted` is true.

Customer Match needs an Ads account with a good policy and payment history, and a list targets only once enough
members match Google accounts. Your privacy notice should say that customers who opt into marketing may be shown ads
on Google through Customer Match. View-through conversions and Consent Mode's modelled conversions depend on a
browser tag, so they aren't available.
