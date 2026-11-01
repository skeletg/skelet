- **A second template, `astro-shop`, built for speed.** An Astro 7 storefront under `templates/` on the
  same front API as nextjs-vetrina, for small catalogs. It has a home page, product pages, a cart and a
  one-page checkout. Pages are rendered once and served from Astro's route cache until the backend purges
  them through `/api/revalidate`, the same contract nextjs-vetrina implements. Every page works without
  JavaScript, and the scripts it does ship come to about 5 KB gzipped. Product JSON-LD, canonical URLs,
  `robots.txt` and `sitemap.xml` are built in. It has a light and a dark theme that follow the system
  until the shopper picks one, and self-hosted Poppins and Montserrat fonts - `?font=montserrat` on any
  URL switches the font for testing. It runs on the same storefront core as nextjs-vetrina, through
  `@skelet/storefront-astro`. Run it with the `astro-shop` profile on port 3001, and test it with
  `skelet test storefront astro-shop`.
