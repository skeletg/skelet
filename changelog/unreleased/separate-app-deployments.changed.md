- **Core and Signals deploy on their own.** Each has a `compose.yaml` and a `.env.example` at its root
  (`apps/backend/core/`, `apps/backend/signals/`) and keeps every setting and secret it reads in that one `.env`, so
  the two can share a box or sit on different servers. `docker-compose.prod.yml` and `docker-compose.small.yml` include
  them for a one-box install, and the root `.env` keeps only what the frontends and the edge read. `env/prod.env`,
  `env/prod-signals.env` and `env/small.env` are gone - `skelet setup` writes the new files and moves backend keys out
  of the root `.env`. Signals runs its own Valkey, and publishes its ports on `SIGNALS_BIND`.
- **`skelet release` builds only the apps you name** - `skelet release core`, `skelet release signals dashboard` - and
  tags them with `git describe` unless `--tag` says otherwise. The core image is `skelet-core`, set as `CORE_IMAGE`.
