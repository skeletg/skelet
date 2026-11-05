- **GA4 gets whole sessions without a script.** The storefront now reports every page view to its own
  `/t/e`, and keeps a first-party analytics session (with analytics consent only). GA4 events then
  carry `session_id`, engagement time and the page title, so they appear in GA4's session reports.
