- **Set up tracking from the dashboard.** Marketing → Tracking lists every platform with its status
  on the channel, including the ones not in this build with the command that adds them. A guided
  setup explains what each platform needs and where to find it, the consent it requires, saves it
  switched off in the platform's test mode, sends a test with the platform's answer, and turns it on.
  Each destination has a delivery log with retry, and credentials stay write-only.
- **Settings → Privacy** brings the cookie consent mode and exceptions together with which
  destinations each consent category feeds.
- **System health** shows the tracking queue across every channel: how many destinations are on,
  how many events failed in the past day, and whether the queue has fallen behind.
- New permissions `tracking.view` and `tracking.manage`.
- **Plugin SDK:** `sdk.DestinationField.TestMode` marks the setting that keeps a destination's
  sends out of the platform's live reports. The dashboard reads it to warn before a test send that
  would count as real, and clears it when the destination goes live.
