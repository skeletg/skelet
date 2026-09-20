# Backups and restore

The runbook for turning a bucket full of dumps back into a running shop.
Read it before you need it; better yet, rehearse it once. A backup you
have never restored is a hope, not a backup.

Every variable named here is documented in
[env.md](env.md#database-backups).

## What the backups are

The `db-backup` scheduler job runs nightly at 03:00 and streams `pg_dump`
straight into the bucket named by `BACKUP_S3_BUCKET`, never to local disk.
`skelet db backup` runs the same code on demand.

- **Three artifacts per run**, sharing one timestamp so a restore can pair
  them: `db/skelet-<ts>.dump` (commerce), `db/skelet-signals-<ts>.dump` (the
  Signals database) and `db/signals-clickhouse-<ts>/` (the event spine).
  Restore all three from the same timestamp.
- **Custom format, zstd-compressed.** `pg_restore` reads the dumps directly.
- **Encrypted with age** when `BACKUP_ENCRYPTION_PASSPHRASE` is set, which
  adds a `.age` suffix to the key. Without the passphrase those objects are
  unreadable by anyone, including you.
- **The spine copy is ClickHouse's own `BACKUP DATABASE`.** The server writes
  it straight into the bucket, a directory of zstd parts, so `BACKUP_S3_ENDPOINT_URL`
  has to resolve from the ClickHouse container. It is not age-encrypted: nothing
  sits between the server and the bucket to encrypt it, so it relies on the
  bucket's own encryption at rest.
- **Retention keeps the newest `BACKUP_KEEP` (default 30) per database.**
  Count, not age: a burst of manual runs prunes older nightlies sooner. The
  spine's directories are pruned by the same count.
- Analytics retention archives share the bucket under
  `<prefix>archive/`. The dump pruner never touches them.

What is **not** in a dump: the search index (OpenSearch is rebuilt, not
restored), media objects (they live in the media bucket), and Valkey
(caches and sessions, all disposable).

## Restore

The commands below run inside the app container, which already has the
credentials and `pg_restore`. `bin/cli` is the dev wrapper; in prod use
`docker compose -f docker-compose.prod.yml exec app skelet ...`.

**1. Pick a dump.** Oldest first, so the newest pair is at the bottom:

```bash
skelet db backups
```

**2. Stop the writers.** Leave the database up, take the app down, or the
restore fights live traffic:

```bash
docker compose -f docker-compose.prod.yml stop app worker media signals ingest
```

**3. Restore both dumps of one timestamp.** This is destructive:
`pg_restore --clean --if-exists` drops and recreates objects, which is why
it demands `--yes`. Encrypted dumps decrypt in flight, and age
authenticates them, so a tampered object fails here instead of restoring
quietly:

```bash
skelet db restore db/skelet-<ts>.dump.age --yes --jobs 4
skelet db restore db/skelet-signals-<ts>.dump.age --yes --jobs 4   # handed to the signals process over the contract
```

`--jobs N` parallelizes the data and index phases and is the difference
between hours and minutes on a large database. The dump spools to a temp
file first, so point `TMPDIR` at a volume with room for one dump if the
container filesystem is small.

**4. Restore the spine.** ClickHouse restores from the bucket itself, into an
empty database, with the same endpoint and credentials the backup used:

```bash
docker compose -f docker-compose.prod.yml exec clickhouse clickhouse-client --user skelet --password "$CLICKHOUSE_PASSWORD" --multiquery --query "
DROP DATABASE IF EXISTS signals;
RESTORE DATABASE signals FROM S3('<BACKUP_S3_ENDPOINT_URL>/<BACKUP_S3_BUCKET>/db/signals-clickhouse-<ts>/', '<BACKUP_S3_ACCESS_KEY_ID>', '<BACKUP_S3_SECRET_ACCESS_KEY>')"
```

**5. Migrate.** A dump can predate the code you are deploying, so the
schemas may be behind:

```bash
docker compose -f docker-compose.prod.yml run --rm migrate
docker compose -f docker-compose.prod.yml run --rm signals-migrate
docker compose -f docker-compose.prod.yml run --rm clickhouse-migrate
```

**6. Start the app back up**, then work the checklist below.

```bash
docker compose -f docker-compose.prod.yml up -d
```

## Post-restore checklist

Do not skip the reindex. It is the one step whose absence looks like
success: pages load, orders are there, and search quietly serves nothing.

- [ ] `/health` returns ok, and `/readyz` is happy
- [ ] **Rebuild the search indexes** - they are not in the dump:
      `skelet search reindex all` (targets: `products`, `blog`, `all`; it
      builds into a fresh index and flips an alias, so the store keeps
      serving throughout, and `--timeout` defaults to an hour)
- [ ] If media was lost too, restore it **before** reindexing, so
      thumbnails resolve
- [ ] Spot-check across the money path: open a product page, an order in
      the admin, and log in as an admin and as a customer
- [ ] Check the scheduler's run history for jobs that fired against the
      half-restored database and want a manual re-run
- [ ] Rotate anything the incident exposed. A dump holds every secret the
      config registry encrypts, and it decrypts with
      `CONFIG_ENCRYPTION_KEY`

## Media

Media objects are not in the dump. The database holds the keys, so a lost
media bucket means intact orders pointing at missing images.

There is no media mirror in the app, on purpose: bucket-to-bucket
replication is something S3, Garage and every managed provider do better,
with credentials the commerce process never holds. Configure it at the
storage layer:

- **Versioning** on the media bucket, so an overwrite or delete is
  recoverable.
- **Replication** to a second bucket or region, ideally append-only so a
  ransacked source cannot propagate deletions.
- To move objects between buckets after the fact, `skelet media-migrate`
  rewrites the stored keys as it copies.

## Verifying that any of this works

Two things, and the second is the one people skip.

**Watch for silence, not just failures.** The scheduler alerts when the
job fails. It cannot alert when the job never ran, because a process that
is down raises nothing. Set `BACKUP_HEARTBEAT_URL` to a dead-man's-switch
monitor: it is pinged only after a fully successful run, so the monitor
alerting on missing pings is what catches a box that stopped backing up
rather than one that failed loudly.

**Fire-drill, quarterly.** Restore into a throwaway container and confirm
the archive loads end to end:

```bash
docker run -d --name restore-drill -e POSTGRES_PASSWORD=drill postgres:18-alpine
docker exec -i restore-drill sh -c 'until pg_isready -U postgres; do sleep 1; done'
# .age dumps: decrypt with `age -d` first, or restore into a scratch
# database with `skelet db restore` and point DATABASE_URL at it
docker exec -i restore-drill pg_restore -U postgres -d postgres --no-owner < /tmp/skelet-<ts>.dump
docker rm -f restore-drill
```

Exit zero means the chain works: dump, upload, encryption, download,
restore.

## Managed Postgres

Use the provider's point-in-time restore as the primary. It beats nightly
dumps on both recovery point and recovery time. These dumps still earn
their place as the provider-independent copy, for account lockout,
provider migration, or losing a region.

Past roughly 100GB, treat physical backups (pgBackRest, WAL-G, or managed
snapshots) as primary and these dumps as the portable weekly export. See
[env.md](env.md) for the job timeout and `--jobs` guidance at that size.

## Dev

Local snapshots are a different tool, deliberately: `bin/db-export`
dumps the dev database into the gitignored `backups/`, and `bin/db-import`
restores one over it after prompting. Neither touches the backup bucket.
