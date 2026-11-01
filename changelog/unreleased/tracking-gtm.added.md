- **GA4 gets whole sessions without a script.** The storefront now reports every page view to its own
  `/t/e`, and keeps a first-party analytics session (with analytics consent only). GA4 events then
  carry `session_id`, engagement time and the page title, so they appear in GA4's session reports.
- **An optional web Google Tag Manager container** (`general/analytics/gtm_container_id`). It loads
  only after the visitor allows a cookie category, with Consent Mode v2, once the page is idle, and
  optionally from your own server-side GTM domain (`general/analytics/gtm_script_origin`). Without
  it the storefront loads no tracking script at all.
