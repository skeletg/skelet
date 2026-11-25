- **A line that is nothing to ship no longer holds an order at "partially fulfilled".** Order
  lines record whether they ship, from their product type at placement. An order is fulfilled
  once every line that ships has shipped, so a service, a download or a custom charge bought
  beside the goods doesn't have to be "shipped" to close it. The admin API's `OrderLine` has
  `requiresShipping`, and the order page says "not shipped" for such a line.
