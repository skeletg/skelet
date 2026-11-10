- **A channel's prices can include tax.** Settings → Identity has a "Prices include tax" switch, and
  the admin API takes `taxInclusivePricing` on a channel. With it on, checkout splits the tax out of
  the prices instead of adding it on top, and the storefront labels the tax line "Includes tax" in
  the cart estimate, at checkout and on the order. Orders already placed keep the way they were
  priced.
