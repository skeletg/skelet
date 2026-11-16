- **Order numbers follow a format of your own.** `sales/orders/number_format` takes `{seq}` (padded with `{seq:6}`),
  `{yyyy}`, `{yy}`, `{mm}` and `{channel}`, per channel, with one count for the store or one per channel and an optional
  yearly restart. The default, `ORD-{seq:6}`, reads as numbers always have, and the count continues where it was. A
  plugin can name orders itself with `RegisterOrderNumberer`, drawing from core's counts. The count is taken outside
  the placement transaction, so busy stores never queue on it.
