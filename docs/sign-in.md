# Signing in through a provider

Admins and shoppers can sign in with an outside OpenID Connect provider - Google, Microsoft Entra, Okta, Auth0,
Keycloak, anything that publishes a discovery document - as well as with their password. It's set up in the
dashboard's settings, with no code.

## Setting it up

| | Admins | Shoppers |
|---|---|---|
| Settings | General → Admin sign-in, for the whole install | General → Customer sign-in, per channel |
| Address to register at the provider | the dashboard's origin + `/login/sso` | the store's address + `/account/sso` |
| Who can sign in | admins who already exist - nobody becomes an admin by signing in | anyone; a first visit opens an account |

At the provider, create an app (a "web application" or "confidential client"), register the address above as its
redirect URI, and copy its client ID and secret into the settings with the provider's issuer URL. The button's
label is yours to word.

The dashboard's origin is one of `CORS_ALLOWED_ORIGINS`, and the store's is its channel's public URL - core refuses
to send a provider anywhere else. A store that serves several languages still registers the one unprefixed
`/account/sso`, and the shopper comes back in the language they started in.

## How someone is matched to an account

The first time, by email - and only an email the provider says it has verified (its `email_verified` claim). From
then on by the provider's own id for them, so an address they change at the provider still reaches the same
account here.

- **Admins** need an admin account with that address already, and an active one. **Email domains** narrows it to
  your company's domains, and then the provider is trusted for those addresses even when it doesn't send
  `email_verified` - Microsoft Entra often doesn't.
- **Shoppers** with an account on that store are signed into it; anyone else gets one, its address already
  verified and no password until they set one through a reset. Registration plugins and webhooks still see it
  (`customer.registering`), and can refuse it.

Either way the sign-in raises the same `auth.login.attempting` and `auth.login.succeeded` events a password
sign-in does, and ends in the same session.

## What keeps it safe

- The authorization code flow with PKCE, a single-use state that expires after ten minutes, and a nonce checked
  against the ID token, whose signature, issuer, audience and expiry go-oidc checks.
- The code comes back to the dashboard or storefront page, which hands it to core with a binding only the browser
  that started holds (session storage in the dashboard, an httpOnly cookie on the store) - so someone can't sign
  you into their account by sending you their own sign-in link.
- The provider's error text goes to core's log, never to the person signing in.
- An admin's second factor is the provider's to ask: signing in through it doesn't ask for their authenticator code.
- Deleting or erasing a customer, or deleting an admin, removes the provider ids linked to them.

## What isn't there yet

One provider per audience (and per channel for shoppers), no way to turn passwords off for admins who use a
provider, and nothing for SAML or a provider that isn't OpenID Connect.
