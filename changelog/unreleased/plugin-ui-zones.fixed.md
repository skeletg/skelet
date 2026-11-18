- **A storefront plugin's slots draw.** Plugins were loaded only by the server layout, whose copy of the registry is
  not the one the storefront's client components read, so no slot a plugin registered ever rendered. The cart's
  actions zone now also gets the subtotal and currency its docs promised.
