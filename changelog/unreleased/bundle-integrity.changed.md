- **Choosing Bundle points to Kit for fixed contents.** The Bundle card on the new-product
  page now says to use Kit when the contents are the same every time.
- **Bundle option styles say what the customer does.** "Selection style" is now "Customer
  picks", with choices such as "Pick one · radio" and "Pick several · checkboxes", instead of
  bare widget names that hid which ones behave the same.
- **A bundle price set by the `cart.bundle.adding` hook is now recorded on the cart line**
  rather than guessed from the line not matching its components' sum. A discounted bundle
  never matches that sum either. Lines already in carts keep the old guess through a backfill.
