- **A kit can let customers return its items one at a time.** A new "Return items separately"
  switch in the kit's section decides whether a kit comes back whole only (the default) or item
  by item, each item refunded at its share of the kit price and restocked on its own. Every order
  keeps the setting it was placed under.
- **A kit order lists what was in the box.** A kit line now carries its items onto the order, so
  the order page and the return dialog show them like a bundle's.
- **The storefront shows what is in a kit's box.** `Product.kitComponents` lists a kit's items
  on the storefront API, and the vetrina-demo product page shows them as "In the box". An item
  the store doesn't sell on its own is listed without a link.
