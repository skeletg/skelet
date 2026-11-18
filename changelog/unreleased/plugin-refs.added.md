- **Plugin types point back at core's records.** A field on a plugin's own type can be a core record - a warranty's
  product, a note's customer - answered from `graph/admin/refs` or `graph/front/refs`, which load core's models through
  core's own queries with their permission checks and storefront visibility, one per id in order.
