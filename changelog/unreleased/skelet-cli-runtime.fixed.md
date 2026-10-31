- **`skelet sh`, `cli` and `provider` find a running app container by its service name.** They
  looked for `skelet-app` in the container list, so with `COMPOSE_PROJECT_NAME` set they never saw it
  and started a second app container every time.
- **`skelet search` needs neither `jq` nor `curl`**, prints its table everywhere, and rejects a
  `--limit` that isn't a number instead of sending OpenSearch broken JSON.
- **A failed `skelet db export` leaves nothing behind**, and `skelet db import` without a terminal says
  to re-run with `-y` instead of failing on `/dev/tty`.
- **`skelet serve` reads the root `.env` the way compose does** rather than running it as a shell
  script.
