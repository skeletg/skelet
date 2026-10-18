# Backups

skelet doesn't take database backups. That's your infrastructure's job, and
this page is about setting it up.

The reasoning is short: a logical `pg_dump` from inside the application works
fine for a small store and quietly stops working as one grows, because the dump
gets slower, holds a snapshot open against vacuum for its whole run, and leaves
you with a restore that rebuilds every index. You find out it outgrew you at
3am. Magento shipped exactly that feature, then deprecated it for the same
reason. So skelet leaves it to tools built for the job.

## What has to be backed up

| | What it holds | How |
|---|---|---|
| Commerce Postgres | Orders, customers, catalog, everything that matters | PITR, below |
| Signals Postgres | Campaigns, flows, segments, the send log | PITR, below |
| ClickHouse spine | Raw analytics events, Lens sessions | `BACKUP DATABASE`, below |
| Object storage | Media, and Lens replay blobs | Bucket replication or versioning, or a snapshot of `STORAGE_DIR` on the filesystem driver |

What you don't need to back up: OpenSearch, which is rebuilt from Postgres with
`./skelet cli search reindex`, and Valkey, which is caches and sessions.

## Postgres

**On managed Postgres** - DigitalOcean, RDS, Cloud SQL, Neon, Supabase - point-in-time
recovery is a checkbox. Turn it on, set the retention window, and you're done.
This is the easy path and it's the one to take if you're on a provider that offers it.

**Self-hosting Postgres** in the compose stack, use [pgBackRest](https://pgbackrest.org/)
or [wal-g](https://github.com/wal-g/wal-g). Both take a physical base backup and
then continuously archive the write-ahead log, which is what lets you restore to
any moment rather than to last night. pgBackRest is the more capable one;
wal-g is a single binary configured by environment variables and is the quicker
start.

Either way you want `archive_mode = on` and an `archive_command` on the Postgres
container, and a destination the database host can't delete from.

Two things to know. Physical backups restore only into the same Postgres major
version and architecture, so keep an occasional `pg_dump` for version upgrades and
for moving hosts. And they work per cluster, not per database: in the split prod
topology commerce and signals are separate containers so they archive independently,
but in the single-container small tier they share one cluster and rewind together.

## ClickHouse

ClickHouse backs itself up with `BACKUP DATABASE signals TO S3(...)`, run on a
schedule of your choosing. It writes a directory of compressed parts straight to
the bucket. Restore is `RESTORE DATABASE signals FROM S3(...)`.

The spine is append-only event data, so losing a day of it costs you analytics
history rather than money. Back it up, but it doesn't need the same RPO as the
commerce database.

## Restore ordering

If you restore both Postgres databases, take them from the same point in time.
Signals holds a pushed copy of core's channels and customers; a suite restored
from further back will have a stale roster, which core's roster-push job repairs
on its next tick. A suite restored from further *forward* than core can reference
customers core no longer has, which is the direction to avoid.

After any restore: `./skelet cli search reindex` to rebuild OpenSearch, and check
that scheduled jobs aren't all overdue on the health page.

## Local snapshots

For development, `./skelet db export` dumps to the gitignored `backups/`
directory and `./skelet db import` restores one. These write local files and
touch no bucket. They're a dev convenience, not a backup strategy.

## The BACKUP_* variables

`BACKUP_S3_BUCKET` and its siblings are **not** database backups, despite the
name. They're the destination for the analytics retention archive: with
`signals/analytics/retention_action` set to `archive`, the retention jobs export
aged spine events and send-log months as identity-stripped `.jsonl.gz` files under
`<prefix>archive/` before deleting the rows. With no destination configured that
sweep fails closed and deletes nothing.
