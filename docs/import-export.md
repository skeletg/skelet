# Import & export

Every bulk transfer in or out of the store goes through one registry, one page and one
CLI command. Nothing is scattered per-feature any more: if a thing can be exported, it's
at `/data` in the dashboard and in `skelet data list`.

The registry is `internal/dataio`. A **resource** declares its columns and a per-row
upsert; the framework owns everything generic - parsing, header validation, dry runs,
per-row error isolation, progress, and the file itself.

## What ships

| Resource | Slug | Direction | Upserts on |
|---|---|---|---|
| Products | `products` | both | `sku` |
| Categories | `categories` | both | `slug` |
| Customers | `customers` | both | `email` + the run's channel |
| Stock levels | `inventory_levels` | both | `sku` + `location` |
| Tax rates | `tax_rates` | both | nothing - every row creates a rule |
| Promotion codes | `promotion_codes` | both | `code` |
| Order lines | `order_lines` | export | - |
| Audience (hashed) | `audience` | export | - |

Order lines and audience are export-only on purpose: an order is never created from a
spreadsheet, and a hashed email can't be turned back into a customer.

## How a run works

Every transfer is a **job**, not a request/response. You queue one, it runs in the
background, and the file lands in object storage.

That matters more than it sounds. A 100k-product export as a GraphQL string is an
allocation bomb; as a job it streams row by row through a pipe into the store and never
exists in memory. It also means closing the tab doesn't kill a half-hour import.

Jobs are claimed with `SKIP LOCKED` and hold a lease, so every replica can run the loop
and a crashed runner's work becomes claimable again once its lease lapses. The lease renewal
doubles as the cancel signal: it only matches a row still marked `running`, so cancelling a
job makes the next renewal touch nothing and the runner stops within a couple of seconds.
A job whose runner died more times than its retry budget allows is swept into `failed` rather
than sitting at `running` forever.

**Rows are independent.** One bad row lands in the error report with its line number and
its key, and the rest still import. A returned *error* means the file was unusable - a
missing key column, unreadable CSV - not that some rows failed.

Retention is two-stage: the generated file is swept after
`system/data_transfer/artifact_retention_days` (7 by default), and the job row outlives
it so the history stays readable after the file is gone.

## Permissions

`data.view` gets you the page and the job history. What you can actually *do* is gated
per resource by its own permission, so the hub can't become a side door:

- Exporting `customers` needs `customers.manage`, because that's PII leaving the system.
- Exporting `order_lines` needs `orders.view` *and* `customers.manage` on the same channel,
  since every row carries the customer's email. A resource can ask for extra grants like
  this through `ViewAlsoRequires`, and every read path checks all of them.
- Importing `products` needs `catalog.manage`.
- The resource list only shows what you hold, and the job list is narrowed the same way -
  a job for a resource you can't read isn't listed, downloadable, or countable.

An all-channel run needs the permission unscoped; a run targeting one channel only needs
it for that channel.

`data.view` is new, so only the Administrator role has it after a deploy. Grant it to
whichever roles should reach the hub.

## Formats

CSV and JSON Lines, chosen per run. Both round-trip: an export re-imports without editing.

Pipe-separated lists (`color|size`) rather than commas, because commas belong to the CSV.
Money is integer minor units - `1999` is 19.99, never `19.99`, because float cents are how
rounding bugs get in.

## The CLI

Same registry, run straight through rather than as a job - an operator at a terminal
already has somewhere to stream to.

```bash
skelet data list                       # every resource
skelet data list products              # its columns, flags and options
skelet data template products          # a header row plus one example row
skelet data export products -o out.csv --channel eu
skelet data import products out.csv --dry-run
```

Customers, promotion codes and audience aren't in the CLI: their services pull in most of
the graph (mailers, consent, the event bus), which a one-shot command doesn't justify.

## Adding a resource

Write a `dataio.Resource` and register it in `internal/wiring/datatransfer.go`. Both
handlers close over live services, which is why registration happens at wiring time rather
than from `init()` - the same reason `internal/adminaction` does.

```go
dataio.Register(dataio.Resource{
    Slug: "widgets", Label: "Widgets", Group: "Catalog",
    ViewPermission: "catalog.view", ImportPermission: "catalog.manage",
    Fields: []dataio.Field{
        {Name: "sku", Type: dataio.FieldText, Identifier: true, Required: true, Example: "W-1"},
        {Name: "price", Type: dataio.FieldMoney, Description: "Minor units"},
    },
    NewExporter: func(ctx context.Context, o dataio.Opts) (dataio.Exporter, error) { ... },
    NewImporter: func(ctx context.Context, o dataio.Opts) (dataio.Importer, error) { ... },
})
```

An importer implements one method:

```go
func (im *widgetImporter) Row(ctx context.Context, r *dataio.Row) (dataio.Outcome, error)
```

Two optional interfaces cover the awkward cases. `Pass(*Row) int` groups rows so an earlier
pass completes before a later one starts - that's how a configurable parent gets created
before the variant rows that link to it. `Finish(ctx) ([]RowError, error)` runs once at the
end for work that needs every row to have landed, and its errors still blame the row that
caused them.

`Row`'s typed accessors (`Str`, `Int`, `OptBool`, `List`, `Enum`, `RequireDeclared`)
accumulate cell errors instead of returning them, so a row reports **every** bad cell at
once rather than one per re-upload. The driver fails a row whose accessors rejected
anything, even if the handler ignored it - a bad price can't silently import as 0.

**Adding a column** to an existing resource is one `Field` entry plus one line in the
mapper. It reaches the export header, the import template, the admin UI's column reference
and validation without touching any of them - `Enum` is enforced by the driver before your
handler runs, and `RequireDeclared()` reads the required set off the descriptor, so neither
can drift from what the UI advertises.

**Adding an option whose choices are entity ids** (a rule, a segment) means declaring
`Source: "your_source"` and registering a provider in `internal/wiring/datatransfer.go`,
mirroring `config.RegisterSource`. The dashboard needs no change - it renders whatever
choices the backend hands it. A `Source` naming nothing fails the boot rather than silently
rendering an empty dropdown.

**Adding a format** is one `Codec` implementation plus a `registerCodec` call. Nothing in
the resources or the driver changes - see `codec_jsonl.go`, which is the whole proof.

## Gotchas worth knowing

- **An export of drifted data won't always re-import.** If a product carries an attribute
  value that isn't assigned to its product type, the exporter faithfully writes the column
  and the importer rejects the row. That's deliberate - failing loudly beats silently
  dropping the value - but it surprises people the first time.
- **Tax rates have no natural key**, so re-running the same file duplicates the rules. It's
  the one importable resource that isn't idempotent, and the column reference says so.
- **`quantity` on stock levels is absolute, not a delta.** The difference is written to the
  ledger as a `manual_adjustment`, so the ledger stays the story of how stock moved.
- **A row that only flips `tracked`** (no `quantity`) is a legitimate row, not a no-op.
- **The import route is exempt from the global request-body cap.** That cap
  (`REQUEST_MAX_BODY_BYTES`, 8MB) is sized for JSON API payloads, and this store's own
  product export is already twice that - so the exemption in `httpx.LimitBody` is what makes
  a file round-trip at all. The route enforces its own 200MB limit and answers 413 past it,
  rather than truncating.
- **New scheduled jobs need the worker restarted.** The sweep is claimed by whichever process
  gets there first; a worker running an older binary reports
  `no handler registered: code job "data-transfer-sweep"` and the sweep silently does nothing.
