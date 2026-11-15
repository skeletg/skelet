- **Signals names events the way GA4 does.** `cart_add` is now `add_to_cart`, `order_placed` is `purchase`,
  `registered` is `sign_up`, `$search` is `search`, and so on for every event GA4 has a name for - the same names
  the tracking plugins send, so a board, a segment and an ad platform all read one vocabulary. Each store migrates
  on boot: events already recorded, saved boards and drafts, segments and experiment goals are renamed, and a
  browser's copy of a purchase is dropped, since the server's order is the one purchase. A storefront older than
  this release still sends `$search`, `product_filter` and the AI overview's old names; Data health lists them until
  it is redeployed. Pulse flow triggers keep their names.
