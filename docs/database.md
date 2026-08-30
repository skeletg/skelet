# Database and migrations

Three separate schemas, three separate streams. Each is applied by its own
`migrate` one-shot in the compose files, and none of them knows about the others.

| Stream | Files | Applied to |
|---|---|---|
| Commerce | `apps/backend/internal/db/migrations` | `DATABASE_URL` |
| Signals | `apps/backend/internal/db/signals/migrations` | `SIGNALS_DB_URL` |
| ClickHouse spine | `apps/backend/internal/analytics/spinech/migrations` | `SIGNALS_CH_MIGRATE_URL`, only under the `clickhouse` profile |

## What the files are

Commerce starts from two baseline files, split along the one line that matters -
structure and the rows that fill it:

- `000001_schema` is every table, constraint, index, type and extension
- `000002_reference_data` is the rows a fresh install starts with - currencies,
  countries, the admin roles and their permission scopes, the default channel and
  locale, customer groups, email groups, the review taxonomy and the built-in
  shipping carrier

Signals starts from `000001_baseline` and seeds nothing. The ClickHouse spine
starts from `000001_spine`.

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

The `migrate` one-shot runs `skelet data-patch up` straight after the schema in all
three compose files, so patches land before any backend role boots. `skelet data-patch
status` lists what has run.

**Reference data does not belong here.** Rows every install needs go in
`000002_reference_data`, where they arrive without anyone running a command. Core ships
no patches for exactly that reason - `internal/datapatch/patches` is an empty package
waiting for the first change that genuinely needs Go. Plugins register theirs through
`sdk.RegisterDataPatch`, and the host bridge attributes each one to its plugin so
uninstall can find them.

## Adding a migration

Write `000003_<what>.up.sql` and its `.down.sql` by hand and leave the baselines
alone. `bin/migrate up` applies it, `bin/codegen` regenerates sqlc from it.
Qualify your table names with `public.` or rely on the search path being intact -
see the guard below for why that is not guaranteed.

## Regenerating a baseline

Only worth doing when the stream has grown a long tail of small migrations that
nobody outside this repo has ever applied. It is a rewrite of history, so
everyone with an existing database has to re-pin afterwards.

1. Apply the current stream to a scratch database
2. `pg_dump --no-owner --no-privileges --schema-only --exclude-table=schema_migrations`
   for the schema half, and the same with `--data-only --inserts` for the seed
3. Strip the `\restrict` / `\unrestrict` lines - those are psql meta-commands and
   golang-migrate is not psql, so it fails on them
4. Strip `SELECT pg_catalog.set_config('search_path', '', false);`, because
   migrate reuses ONE connection for the whole run and an empty search path
   leaks into every migration after it. `internal/db/baseline_guard_test.go`
   fails if a regenerated dump brings it back
5. Rebuild a second scratch database from the new files and diff a full
   `pg_dump` of both - they must be byte-identical
6. `bin/sqlc generate` must produce no drift, since `sqlc.yaml` reads the
   migration directories as its schema source
7. Re-pin every existing database with `migrate force <new latest version>`,
   and drop the e2e databases so `bin/test-e2e --fresh` rebuilds them

The Signals baseline needs one extra step. `email_sends` is RANGE-partitioned by
month, so a dump freezes whichever months existed when it was taken. Drop every
`email_sends_YYYYMM` block from the dump and end the file with
`SELECT public.ensure_email_send_partitions(3);` instead, so a fresh install gets
months relative to its own install date. The daily analytics prune calls the same
function to keep running ahead of the data.

ClickHouse migrations are hand-written, not dumped. The driver splits
multi-statement files on `;` without parsing comments, so a semicolon inside a
comment cuts the file in half - keep them out.
