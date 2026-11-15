- **Admins and shoppers can sign in through an outside provider.** Any OpenID Connect provider - Google, Microsoft,
  Okta and the rest - set up in the dashboard with no code: one for the dashboard's admins, and one per channel for
  its shoppers. Admins must already have an account, optionally limited to your company's email domains; a shopper's
  first visit opens an account. Accounts are matched on a verified email the first time and on the provider's own id
  after that. See [sign-in.md](../../docs/sign-in.md).
