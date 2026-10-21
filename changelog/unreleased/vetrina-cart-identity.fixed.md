- **Vetrina edits the shopper's own cart, and no other.** Signed-in shoppers can change, remove,
  save and estimate cart lines without a cart cookie, because every cart action now resolves
  through the customer session or the guest cookie. Only adding to the cart creates a guest cart,
  so a coupon link without a cart waits for the first addition. A backend failure fails the
  action instead of switching to another cart. The cart in the browser also reloads after sign-in
  merges the guest cart, and after an edit fails.
