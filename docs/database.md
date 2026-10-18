# Database and migrations

Three separate schemas, three separate streams. Each is applied by
[dbmate](https://github.com/amacneil/dbmate) from its own one-shot in the compose files,
and none of them knows about the others.

| Stream | Files | Applied to |
|---|---|---|
| Commerce | `apps/backend/core/internal/db/migrations` | `DATABASE_URL` |
| Signals | `apps/backend/signals/internal/db/migrations` - the suite's, holding Pulse (campaigns, flows, the email builder) beside the spine | `SIGNALS_DB_URL` |
| ClickHouse spine | `apps/backend/signals/internal/analytics/spinech/migrations` | `SIGNALS_CH_URL`, applied by the `clickhouse-migrate` one-shot wherever the suite runs |

## What the files are

Commerce starts from two baseline files, split along the one line that matters -
structure and the rows that fill it:

- `<timestamp>_schema` is every table, constraint, index, type and extension
- `<timestamp>_reference_data` is the rows a fresh install starts with - currencies,
  countries, the admin roles and their permission scopes, the default channel and
  locale, customer groups, the review taxonomy and the built-in shipping carrier

Signals is split the same way: its `_schema` is the suite's whole schema - the spine,
Lens, the rosters, segments, experiments, the send log and Pulse - and its
`_reference_data` is the two system email groups the builder files emails under.
Nothing else is seeded; the starter content is the operator's to ask for from the
emails hub. The ClickHouse spine starts from one hand-written `_spine` file.

Both Postgres baselines are generated, not hand-written. Don't edit them - change
the schema with a new migration on top, or regenerate as below. The file you read
to understand a table is `internal/db/query/*.sql` and the sqlc models beside it,
not the dump.

## Data patches

`internal/datapatch` is the run-once counterpart for changes that need Go the schema
cannot express - walking a block tree to build a search vector, encrypting with a key
from the environment, anything that has to read a code-defined registry. Each patch
declares an `ID`, an optional `DependsOn`, and an `Apply`; the runner topologically
sorts them and commits each patch together with its `data_patches` marker, so a failure
rolls back and retries next deploy.

The `migrate` one-shot runs core's migrations, then `skelet plugin migrate up`, then
`skelet data-patch up` in all three compose files, so a patch can count on every schema
it touches and lands before any backend role boots. Runs that overlap - a hand-run `up`
during a deploy - take turns per patch, and the second finds it already applied.
`skelet data-patch status` lists what has run.

**Reference data does not belong here.** Rows every install needs go in
the reference-data baseline, where they arrive without anyone running a command. Core ships
no patches for exactly that reason - `internal/datapatch/patches` is an empty package
waiting for the first change that genuinely needs Go. Plugins register theirs through
`sdk.RegisterDataPatch`, and the host bridge attributes each one to its plugin so
uninstall can find them.

## Adding a migration

`./skelet db migrate new <what>` (add `--signals` for the suite's stream) makes
`<timestamp>_<what>.sql` with its two halves:

```sql
-- migrate:up
ALTER TABLE public.orders ADD COLUMN gift_note text;

-- migrate:down
ALTER TABLE public.orders DROP COLUMN gift_note;
```

Fill both in and leave the baselines alone. `./skelet db migrate up` applies it,
`rollback` undoes the highest version, `status` lists them, and `./skelet gen`
regenerates sqlc from it - sqlc reads only the up halves. Each migration runs in a
transaction; write `-- migrate:up transaction:false` for the rare statement that can't,
like `CREATE INDEX CONCURRENTLY`. Qualify your table names with `public.` or rely on the
search path being intact - see the guard below for why that is not guaranteed.

Migrations are timestamped, so two branches never pick the same name. dbmate records every
version it applies, not just the highest, so a migration that lands after a newer one - two branches merged out of order -
is still applied rather than skipped. `dbmate up --strict` refuses one instead, for an
environment that should never see it happen.

## Regenerating a baseline

Only worth doing when the stream has grown a long tail of small migrations that
nobody outside this repo has ever applied. It is a rewrite of history, so
everyone with an existing database has to re-pin afterwards.

1. Apply the current stream to a scratch database
2. `pg_dump --no-owner --no-privileges --schema-only --exclude-table=schema_migrations`
   for the schema half, and the same with `--data-only --inserts` for the seed
3. Strip the `\restrict` / `\unrestrict` lines - those are psql meta-commands and
   dbmate is not psql, so it fails on them. Put the schema under `-- migrate:up` and
   a `DROP SCHEMA public CASCADE; CREATE SCHEMA public;` under `-- migrate:down`
4. Strip `SELECT pg_catalog.set_config('search_path', '', false);`, because
   a migration run keeps one session and an empty search path
   leaks into every migration after it. `internal/db/baseline_guard_test.go`
   fails if a regenerated dump brings it back
5. Rebuild a second scratch database from the new files and diff a full
   `pg_dump` of both - they must be byte-identical
6. `./skelet gen sqlc generate` must produce no drift, since `sqlc.yaml` reads the
   migration directories as its schema source
7. Re-pin every existing database: delete the squashed versions from
   `schema_migrations` and insert the new baseline versions, leaving the rows of
   whatever sits on top alone. Then `./skelet db migrate up`. Drop the e2e databases
   so `./skelet test e2e --fresh` rebuilds them

The signals stream needs one extra step. `email_sends` is RANGE-partitioned by
month, so a dump freezes whichever months existed when it was taken. Drop every
`email_sends_YYYYMM` block from the dump - the tables, their constraints and indexes,
and the `ATTACH PARTITION` statements for each - and end the file with
`SELECT public.ensure_email_send_partitions(3);` instead, so a fresh install gets
months relative to its own install date. The send-log retention sweep calls the
same function to keep running ahead of the data.

ClickHouse migrations are hand-written, not dumped. ClickHouse takes one statement
per query, so give every statement its own `-- migrate:up` / `-- migrate:down` pair in
the file - dbmate runs each section as a query of its own. A section with nothing to undo
still needs a statement under its `-- migrate:down`; `SELECT 1;` does.
