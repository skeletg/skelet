- **An order placed without analytics consent no longer joins a shopper's profile in Signals.** It
  still counts in revenue, but under the order alone: no account, visitor id or email.
- **The backend now sees the shopper's own browser.** The storefront forwards their user agent, so
  session lists and bot checks show the shopper's browser instead of the storefront server.
- **A signed-in shopper who refused analytics no longer has cart activity on their Signals
  profile.** A guest's checkout opt-in for an address whose account is already verified now
  applies straight away.
