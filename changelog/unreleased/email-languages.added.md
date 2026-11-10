- **Emails in the shopper's language.** A library email can have a version per language, written
  from the language picker in its settings - a new one opens as a copy of the default. Order emails
  go out in the language the order was placed in, account emails in the one the shopper is browsing,
  and campaigns, marketing flows and cart offers in the one the customer last signed in with. A
  regional locale reads its language's version, and everything else gets the email as written.
  Orders, customers and pending newsletter signups now keep that language (`orders.locale`,
  `customers.locale`, `signup_confirmations.locale`).
