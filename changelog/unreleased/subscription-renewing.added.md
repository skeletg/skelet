- **Plugins can shape each subscription renewal.** `subscription.renewing` fires as a period is invoiced, so a plugin
  or a sync webhook can change what it costs, skip it, or move it to a later date. A period made free is marked paid
  without a charge.
- **The retry schedule for failed renewals is a setting.** `sales/subscriptions/retry_days` takes the days between
  retries per channel - 1, 3, 7 as before, or 0 to cancel at the first failure.
