- **The demo storefront no longer records shoppers' private pages.** Session replay and heatmap
  snapshots skip account, cart, checkout, order, order-tracking, gift-card, shared-wishlist and
  preview pages, with or without a locale prefix, and the signed-in name in the header, the
  masquerade banner and the sign-in drawer are blanked everywhere. Page views and click heat
  still count those pages. Replay pauses on a private page and resumes with a fresh snapshot on
  the next public one.
