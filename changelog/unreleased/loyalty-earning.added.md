- **Plugins can set the loyalty points an order earns.** `loyalty.earning` fires before a paid-for order's points are
  written, with the channel's rate already applied and the order's lines alongside, so a plugin or a sync webhook can
  give double points on a category or none on sale items. A note it adds shows in the points history next to the
  order number.
