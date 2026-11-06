- **An order placed without analytics consent now counts as nobody in Signals**, instead of under an
  id of its own. Cart activity from such a visitor counts the same way instead of being dropped.
  - `skelet cli signals backfill-orders` skips these orders. Each already counts, under its old id
    or as nobody, and replaying it would count it twice.
- **An order keeps a visitor id only when the shopper allowed analytics.** Before, it stored any id
  the request carried, even when the order's own consent snapshot said no.
