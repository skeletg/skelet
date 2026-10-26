- **No `pnpm audit` findings in either app.** body-parser 2.4.0 (a denial of service on compressed
  request bodies) and fast-uri 3.1.9 (four URI parsing advisories), both already inside their
  parents' ranges. Only dev tooling reaches them - the shadcn CLI and Sentry's webpack plugin -
  so nothing that ships was exposed.
