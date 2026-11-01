- Tracking delivery retries now stop at the configured attempt limit when a provider
  panics, and panic logs redact account credentials.
- Workers claim only deliveries they can start immediately, avoiding duplicate sends
  caused by queued work waiting past its claim lease.
