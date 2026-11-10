- **Table rates for shipping.** A shipping method can be a table rate: rows priced by the
  package's weight, the order subtotal or the number of items, where a package pays the last row
  it reaches and isn't offered below the first. A row marked not offered caps the method, items
  with their own product-type rate keep it, and a subtotal table with a free row feeds the
  free-shipping hint. The admin API takes `calcType: TABLE` with `tableBasis` and `rateTiers`.
