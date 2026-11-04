- **Order lines on the analytics spine.** Each placed order records an `order_line` per line with
  its SKU, quantity and net sales (after its share of the order's discount, without tax or
  shipping). Each refunded line records a `refund_line` and each changed line an
  `order_line_adjusted`, with negative amounts on the day they happen, so a product's net sales
  is their sum and last month's figures don't move.
- **Amounts in the base currency.** Order totals, order lines, refunds and attributed conversion
  revenue also carry the amount in the platform's base currency at the day's rate (`*_base_cents`).
- Cart updates and removals now say which SKU they were.
- `skelet cli signals backfill-orders` also replays past order lines and refunded lines. Edits made
  before this release can't be replayed.
