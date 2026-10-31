- **`skelet test e2e` says it needs ClickHouse before it touches anything.** Under `skelet up --lean`
  it dropped and recreated the e2e Postgres databases, then stopped on a compose error about a
  service that wasn't running.
- **`skelet help <command> <sub>` describes the subcommand.** `skelet help test e2e` and
  `skelet help check pins` printed the parent command's help.
