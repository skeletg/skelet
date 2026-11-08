- **Signals event destinations are gone.** The Event destinations page, its webhook audience sync,
  the suite's forwarding job and the `event_destinations` table are removed. Shop events reach GA4,
  Meta, Google Ads, webhooks and the rest from Marketing → Tracking, so set those up there before
  upgrading - the old destinations' secrets don't carry over. Signals' own email events are no
  longer forwarded anywhere.
- **`SIGNALS_ENCRYPTION_KEY` is no longer read.** It only sealed those destinations' secrets, so
  drop it from the root `.env` and `env/signals.env`.
