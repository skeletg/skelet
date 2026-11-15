- **`skelet setup` sets up a server per app.** Name the apps that run on the machine - `skelet setup core signals`,
  `skelet setup signals`, `skelet setup dashboard shop` - and each gets its env file, with nothing else assumed to run
  there. Core makes the values it shares with Signals, the storefronts and the dashboard; on a machine without core,
  setup takes them from core - `--shared` with a copy of core's `.env`, what the app already holds, or pasted at the
  prompt - and refuses with `--yes` rather than make them up. `--small` replaces the small target, and the dev, small
  and prod choice is now dev or server. A dashboard's absolute `VITE_GRAPHQL_URL` keeps its host when the admin API's
  path changes.
