- **A price set on an offer line survives picking its options again.** Re-adding a configured
  or bundle line overwrote the admin's price while the line still said it was set.
- **A line's price can't be set past what the API can carry.** An offer line, a custom line
  and a draft line are held to the same ceiling, and a negative draft price is still refused.
