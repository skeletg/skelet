- **"More like this" for the assistant and shoppers' agents.** A new `find_similar_products` tool finds the store's
  products nearest in meaning to one to five others - by id, SKU, link or exact name, a variant standing for its
  product - from the vectors already stored, so it costs no provider call.
- **An attribute can say similar products must match it.** Its page under Catalog has a **Must match for similar
  products** switch: on for Gender, a men's jacket never answers "more like this dress"; on for Brand, the answer stays
  in the brand. With several products a value any of them has counts, and variants' values count as their product's.
  Off for every attribute until switched on.
- **A value can match every other value of its attribute.** Unisex for Gender, One size for Size, Multicolour for
  Colour: switch on **Matches every value** in the value's row, and similar products held to that attribute match it
  whatever the other side has.
