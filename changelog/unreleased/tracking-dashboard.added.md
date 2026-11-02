- **Set up tracking from the dashboard.** Marketing → Tracking lists every platform with its status
  on the channel, including the ones not in this build with the command that adds them. A guided
  setup explains what each platform needs and where to find it, the consent it requires, saves it
  switched off in the platform's test mode, sends a test with the platform's answer, and turns it on.
  Each destination has a delivery log with retry, and credentials stay write-only.
- **Settings → Privacy** brings the cookie consent mode and exceptions together with which
  destinations each consent category feeds.
- **Import from Signals** copies the old Signals GA4, Meta and webhook destinations, switched off
  until their secrets are entered again. Their event filters don't carry over, and an imported
  webhook receives one event per POST signed with `X-Webhook-Signature-256`.
- New permissions `tracking.view` and `tracking.manage`.
