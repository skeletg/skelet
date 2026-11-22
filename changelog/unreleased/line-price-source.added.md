- **A line's price says where it came from.** A price an admin sets on an offer line, a custom
  line or a draft line keeps who set it, an optional note saying why, and the catalog price it
  replaced - on the cart and on the order line it becomes. The admin API has `priceSource`,
  `listUnitPrice` and `priceReason` on `AdminCartLine` and `OrderLine`, `adminSetCartLinePrice`
  takes a `reason`, and the order page shows the catalog price and the note under a set price.
