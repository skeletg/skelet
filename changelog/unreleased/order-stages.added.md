- **Plugins can add order stages.** `RegisterOrderStage` adds a step inside core's statuses, like "awaiting stock"
  inside confirmed and processing, without touching the statuses payment and stock follow. The dashboard shows an
  order's stage beside its status, filters the order list by it and lets an admin move an order between stages;
  the plugin moves its own with `SetOrderStage`. A stage clears when the order moves to a status it doesn't list,
  and `order.stage_changed` fires either way.
- **`order.transitioning` can refuse an order's move.** It fires before every status or stage change someone asks
  for, from an admin, a shopper's cancel, a shipment or a plugin, to plugins and sync webhooks alike.
