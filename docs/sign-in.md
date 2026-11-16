# Signing in through a provider

Admins and shoppers can sign in with an outside OpenID Connect provider - Google, Microsoft Entra, Okta, Auth0,
Keycloak, anything that publishes a discovery document - as well as with their password. It's set up in the
dashboard's settings, with no code.

## Setting it up

| | Admins | Shoppers |
|---|---|---|
| Settings | General → Admin sign-in, for the whole install | General → Customer sign-in, per channel |
| Address to register at the provider | the dashboard's origin + `/login/sso` | the store's address + `/account/sso` |
| Who can sign in | an admin who connected their provider account under Preferences | anyone; a first visit opens an account |

At the provider, create an app (a "web application" or "confidential client"), register the address above as its
redirect URI, and copy its client ID and secret into the settings with the provider's issuer URL. The button's
label is yours to word. For Microsoft, use your tenant's own issuer (`https://login.microsoftonline.com/<tenant id>/v2.0`):
the shared `common` and `organizations` ones would let in any Microsoft account, so they're refused.

Everything but the label decides who can sign in, and a provider can vouch for anyone, so only a superuser can
change these settings. Changing the issuer clears the client secret, so a secret meant for one provider is never
sent to another - enter the new one with it. A secret is used only with an issuer set at the same level: a channel
that sets its own issuer needs its own secret, rather than inheriting the store's.

The dashboard's origin is one of `CORS_ALLOWED_ORIGINS`, and the store's is its channel's public URL - core refuses
to send a provider anywhere else. A store that serves several languages still registers the one unprefixed
`/account/sso`, and the shopper comes back in the language they started in.

## How someone is matched to an account

A shopper is matched the first time by email - and only an email the provider says it has verified: its
`email_verified` claim, or Microsoft Entra's `xms_edov`, which you add as an optional claim on the app. From then on
by the provider's own id for them, so an address they change at the provider still reaches the same account here.

- **Admins** are never matched by email. Each admin signs in with their password (and two-factor) and connects
  their provider account under **Preferences**; from then on that account signs them in, and they can disconnect it
  there. **Email domains** narrows which provider accounts can be connected, and is checked again at every sign-in,
  so narrowing it shuts out accounts connected before.
- **Shoppers** with an account on that store are signed into it; anyone else gets one, its address already
  verified and no password until they set one through a reset. Registration plugins and webhooks still see it
  (`customer.registering`), and can refuse it, and `auth.login.attempting` runs before anything is opened or claimed.
  A disabled account stays disabled. An account whose address was never verified is handed to the
  provider's shopper: whatever password it had is removed and its sessions end, since whoever set them never
  proved the address was theirs.

Either way the sign-in raises the same `auth.login.attempting` and `auth.login.succeeded` events a password
sign-in does, and ends in the same session.

## What keeps it safe

- The authorization code flow with PKCE, a single-use state that expires after ten minutes, and a nonce checked
  against the ID token, whose signature, issuer, audience and expiry go-oidc checks.
- The code comes back to the dashboard or storefront page, which hands it to core with a binding only the browser
  that started holds (session storage in the dashboard, an httpOnly cookie on the store) - so someone can't sign
  you into their account by sending you their own sign-in link.
- The provider's error text goes to core's log, never to the person signing in.
- An admin with two-factor on still enters their code after the provider, the same as after a password. Turn on
  **The provider asks for a second factor** when your provider enforces one, and an ID token that says so (an
  `amr` claim naming `mfa`, or two different methods) skips it. One method alone - a one-time code, a key - doesn't.
- The provider's addresses are reached only on public IPs (unless `ALLOW_PRIVATE_WEBHOOK_TARGETS` is on, or in
  development), and its answers are capped at a megabyte. The sign-in and token addresses its discovery document
  names are held to https, like the issuer, and the dashboard checks again before it sends the browser anywhere.
- The provider returns only to `/login/sso` on a dashboard origin or `/account/sso` on the store's address.
- Something failing on this side shows as "isn't working right now", with the detail in core's log.
- Deleting or erasing a customer, or deleting an admin, removes the provider ids linked to them.

## What isn't there yet

One provider per audience (and per channel for shoppers), no way to turn passwords off for admins who use a
provider, and nothing for SAML or a provider that isn't OpenID Connect.
