- **Migrations run as the backend boots, with no migrate containers.** The server applies the
  commerce schema, the plugins' migrations and the data patches before it serves; the signals
  and ingest processes do the same for the Signals database and the ClickHouse spine. Processes
  booting together take turns on a lock, so each migration runs once, and a boot refuses to start
  if a migration it applied has since been edited. `skelet migrate` runs the commerce step by hand
  for a deploy that wants it first, and the prod images no longer ship dbmate.
