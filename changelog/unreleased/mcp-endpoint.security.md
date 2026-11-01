- **The AI chat can only run the tools it was offered.** With customer data turned off in the AI
  settings, the chat hid the customer-record tools, but the model could still run one by asking for
  it by name. Now any tool it wasn't offered is refused.
- **Turning off customer data in the AI settings hides emails on orders too.** The order tools kept
  showing each customer's email after the switch was off. They don't now.
- **The AI chat quotes what shoppers and visitors wrote.** Customer names, review summaries, site
  searches, page addresses, referrers, rage-clicked element text, JS errors and custom event names
  reach the model in quotes, with line breaks escaped, so a crafted name or search can't pass itself
  off as part of the tool's answer.
- **The AI chat no longer confirms an id belongs to another channel.** Looking up an order or
  customer from another channel answered differently from one that doesn't exist; both read the same
  now.
