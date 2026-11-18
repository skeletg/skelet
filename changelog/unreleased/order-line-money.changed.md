- **Each order line keeps its own discount and tax.** A promotion on certain products now
  stays on their lines instead of being spread over the whole order, so a line that was never
  discounted is taxed and refunded at what it actually cost, and products at different tax
  rates are refunded at their own. The admin API's `OrderLine` has `discountAmount` and
  `taxAmount`, and the order page shows them under each line's total.
