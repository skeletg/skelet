- **`OrderLine.components` is no longer bundle-only.** Kit lines placed from now on list their
  items there too, so anything reading it (API clients, webhooks, plugins) should not take a
  non-empty list to mean a bundle. A kit item's stored bundle option and selection are empty.
  Kit orders placed before this keep no items and return whole.
- **Editing a kit line's quantity on an order rescales its items**, so the items shown and
  returnable follow the new quantity.
