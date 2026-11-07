- **What the shopping assistant costs and sells.** Every assistant turn is counted per store with the
  tokens it used, and the cart it fills is marked, along with a cart a shopper's own agent handed off,
  so an order placed within 30 days of that help says who helped (`orders.assisted_by`). A **Shopping assistant** section on
  the dashboard Home (`assistantReport` in the admin API) puts turns, carts filled, assisted orders and
  revenue beside the model cost, priced from **Settings, AI, Usage pricing**. Turns also land on the
  Signals spine as `assistant_turn`, and `order_placed` carries `assisted_by`, so funnels and segments
  can use them.
- **The assistant says what it's doing while it works.** Vetrina shows each step as it happens -
  searching for something, looking at a product, checking delivery to a country, updating the cart -
  over a new streaming endpoint, `/api/front/assistant`, that answers like `askShopAssistant` with a
  line per tool call before the reply. A guest keeps the cart the assistant fills even if they close the
  chat before it answers.
