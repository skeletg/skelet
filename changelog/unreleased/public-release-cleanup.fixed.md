- **Browser uploads work behind the shipped CSPs.** The dashboard, storefront and Caddy policies
  only allowed connections to their own origin, so every upload to object storage was refused.
  `UPLOAD_ORIGINS` in the root `.env` names the hosts uploads go to and joins `connect-src` in all
  three; dev allows its local hosts on its own. `./skelet release` now passes a Dockerfile's build
  arguments through from your shell or the root `.env`, which it never did.
- **The demo storefront's tab icon is Skelet's**, not the Vercel triangle create-next-app left behind.
