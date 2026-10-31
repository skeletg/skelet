- **A prod setup starts its datastores.** Setup created the root `.env` empty, without the template's
  `COMPOSE_PROFILES=bundled`, so compose left the bundled Postgres, pgbouncer, Valkey and ClickHouse
  out of the deployment. A new `.env` for small or prod now starts from `.env.example`.
- **Small and prod pass the storefront revalidation secret.** Compose hands the app and the storefront
  `STOREFRONT_REVALIDATE_SECRET` from the root `.env`, which setup never filled, so both got an empty
  one over the value generated into the backend's env file. Setup now copies it across like the
  other shared secrets.
- **The dev storefront signs revalidations with the secret core checks.** It had a fixed value while
  setup generated core's; it now reads core's from the root `.env`, which setup fills.
- **Setup no longer rewrites `plugins.json` in the old format.** It only understood bare `0`/`1`
  entries, so a run dropped module plugins from the file and could write the name of a parked
  `_plugin` directory, which the build then refused. It now merges its gateway picks in the form
  `skelet cli plugin` writes, removes parked names an older run left, and leaves the rest alone.
- **`skelet setup --profile FILE` leaves FILE alone.** It ran the file as a shell script to read it and
  wrote the answers back into it; it now only reads it, and saves the answers to `.skelet-profile`.
- **A `skelet setup --yes` re-run keeps the frontends where they run.** With a profile from before the
  Docker-or-host question, it emptied `.skelet-apps`, moving every containerized app to the host.
- **`skelet add channel` suggests a setup command that keeps your other apps.** It built the `--apps`
  list from `.skelet-apps`, which only names the apps running in Docker.
