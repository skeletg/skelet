- **The scripts in `bin/` behind `down`, `stop`, `restart`, `ps`, `logs`, `serve`, `sh`, `cli`,
  `psql`, `db`, `search`, `release`, `provider` and `fixperms` are gone** - those commands are part of
  the skelet CLI now. Run them as `skelet <command>`: the old `psql` script is `skelet psql`,
  `opensearch` is `skelet search`, and `push-images` is `skelet release`.
