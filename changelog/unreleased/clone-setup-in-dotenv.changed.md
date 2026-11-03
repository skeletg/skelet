- **A clone's setup lives in its `.env`.** `skelet setup` records the target, the apps and on
  dev the compose profiles there - `SKELET_TARGET`, `SKELET_APPS`, `COMPOSE_PROFILES` - instead of
  `.skelet-profile`, `.skelet-apps` and `.skelet-tools` in the repo root, and a re-run takes its
  defaults from the files as they stand rather than from a record of the last answers. A bare
  `docker compose up` now starts the same dev stack `skelet up` does. The first `skelet` run on an
  older clone moves the three files into `.env`. `skelet rm channel` drops a shop from both app
  lists, where it used to leave the saved answers naming it.
