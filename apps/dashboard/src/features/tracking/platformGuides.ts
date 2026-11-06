// What the guided setup says about each platform - an intro, where to go in the vendor's own UI, and the setting
// that makes a test send a test. Field help (where to find each ID) comes from the plugin itself through the API;
// this only adds what a field can't say. A kind missing here gets the generic guide

export const TRACKING_DOCS_URL = 'https://github.com/skeletg/skelet/blob/main/docs/tracking.md'
export const PRIVACY_DOCS_URL = 'https://github.com/skeletg/skelet/blob/main/docs/privacy.md'

export type PlatformGuide = {
  intro: string
  links: { label: string; url: string }[]
  /** About the platform's test-mode setting, which the plugin marks: what it does, and the value a new destination
   *  starts with (empty when it needs one the admin copies from the platform) */
  testMode?: { preset: string; note: string }
}

const GENERIC: PlatformGuide = {
  intro: 'Fill in the settings below, send a test, then turn it on.',
  links: [],
}

const GUIDES: Record<string, PlatformGuide> = {
  ga4: {
    intro: 'Sends page views, product views, cart changes, checkouts and orders to Google Analytics 4 through the Measurement Protocol - no script on the storefront. Point the endpoint at a server-side Tag Manager container to fan the same events out from there.',
    links: [{ label: 'Google Analytics', url: 'https://analytics.google.com/analytics/web/' }],
    testMode: { preset: 'true', note: "Google's validation server checks each event and records nothing" },
  },
  google_ads: {
    intro: "Reports each purchase to Google Ads as a conversion, with the ad click it came from and the shopper's hashed email and phone, through Google's Data Manager API. Refunds restate the value.",
    links: [
      { label: 'Google Ads conversions', url: 'https://ads.google.com/aw/conversions' },
      { label: 'Enable the Data Manager API', url: 'https://console.cloud.google.com/apis/library/datamanager.googleapis.com' },
    ],
    testMode: { preset: 'true', note: 'Google checks the conversion and records nothing' },
  },
  google_ads_audience: {
    intro: 'Keeps a Customer Match list of customers with an account who opted into marketing, so Google Ads can reach them - and removes them when they opt out or their account is deleted.',
    links: [
      { label: 'Google Ads audience manager', url: 'https://ads.google.com/aw/audiences/management' },
      { label: 'Customer Match terms', url: 'https://support.google.com/adspolicy/answer/6299717' },
    ],
    testMode: { preset: 'true', note: 'Google checks the upload and adds no one' },
  },
  meta_capi: {
    intro: "Sends purchases, carts, checkouts and sign-ups to Meta's Conversions API. Events share their id with a browser pixel, so Meta counts each once if you run both.",
    links: [{ label: 'Meta Events Manager', url: 'https://business.facebook.com/events_manager2' }],
    testMode: { preset: '', note: 'Paste the code from Events Manager → Test events; events then show there and nowhere else' },
  },
  bing_uet: {
    intro: 'Sends conversions to Microsoft Advertising through the UET Conversions API, with the Microsoft click id.',
    links: [{ label: 'Microsoft Advertising', url: 'https://ads.microsoft.com/' }],
  },
  openai: {
    intro: 'Sends conversions to OpenAI Ads, with the click reference kept from the ad.',
    links: [],
    testMode: { preset: 'true', note: 'OpenAI checks the event and records nothing' },
  },
  cj: {
    intro: 'Reports sales to CJ Affiliate server to server, with the click its cookie kept.',
    links: [{ label: 'CJ Account Manager', url: 'https://members.cj.com/' }],
  },
  awin: {
    intro: "Reports sales to Awin's Conversion API, with the click its cookie kept and a commission group per item.",
    links: [{ label: 'Awin', url: 'https://ui.awin.com/' }],
    testMode: { preset: 'true', note: 'Awin marks the transaction as a test' },
  },
  klaviyo: {
    intro: "Sends shop events to Klaviyo for flows and segments - only for shoppers whose email opted into marketing, and never a phone number.",
    links: [{ label: 'Klaviyo API keys', url: 'https://www.klaviyo.com/settings/account/api-keys' }],
  },
  attentive: {
    intro: 'Sends product views, cart adds and purchases to Attentive - only for shoppers whose email opted into marketing.',
    links: [{ label: 'Attentive', url: 'https://ui.attentivemobile.com/' }],
  },
  webhook: {
    intro: 'Posts each event as signed JSON to your own endpoint - one POST per event, signed with X-Webhook-Signature-256.',
    links: [{ label: 'Webhook format', url: TRACKING_DOCS_URL }],
  },
}

export function guideFor(kind: string): PlatformGuide {
  return GUIDES[kind] ?? GENERIC
}

export const PURPOSE_LABELS: Record<string, string> = {
  analytics: 'Analytics',
  ads: 'Advertising',
  personalization: 'Personalization',
}

export const EVENT_LABELS: Record<string, string> = {
  page_view: 'Page view',
  view_item_list: 'Product list view',
  select_item: 'Product click',
  view_item: 'Product view',
  view_cart: 'Cart view',
  add_to_cart: 'Add to cart',
  remove_from_cart: 'Remove from cart',
  begin_checkout: 'Checkout started',
  add_shipping_info: 'Shipping chosen',
  add_payment_info: 'Payment chosen',
  purchase: 'Purchase',
  refund: 'Refund',
  sign_up: 'Sign-up',
  marketing_opt_out: 'Marketing opt-out',
}

export const ALL_EVENTS = Object.keys(EVENT_LABELS)

export const eventLabel = (name: string) => EVENT_LABELS[name] ?? name
