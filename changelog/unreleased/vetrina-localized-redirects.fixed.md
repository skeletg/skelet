- **Vetrina account actions keep the shopper's language.** Signing in or registering, password
  resets, verification emails, addresses, subscriptions, saved cards, loyalty redemptions, reviews
  and logout redirected to unprefixed paths, which sent a German shopper back to English. They now go
  to the localized path. Return destinations keep their query, and a path that already carries the
  locale, such as `/de?welcome=1`, no longer gets a second prefix.
