- **A discount code the shopping agent applied now survives the checkout link.** Redeeming a
  handoff copied only the cart lines, so a code the agent quoted the shopper was gone at checkout.
  It now comes along with the lines, and checkout still re-checks it.
- **A shopper-chosen bundle amount keeps the item's quantity step.** A customer-quantity selection
  only had to be above zero, so a direct API call could put a quarter of an item sold whole into a
  bundle. It is now refused with `ErrQuantityOutOfBounds`, at add and at checkout.
