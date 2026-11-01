# Catalogue delivery to third parties

Catalogue plugins share one product reader and the existing destination accounts and
delivery queue. Each adapter owns its format and upload protocol. Add a provider without
adding another catalogue query, scheduler, credential table or retry loop.

| Destination kind | Plugin | Format and transport |
|---|---|---|
| `catalog_openai` | `feed-openai` | Gzipped JSONL, OpenAI discovery schema, SFTP |
| `catalog_webhook` | `feed-webhook` | Normalized JSONL, HMAC-signed HTTP POST |

These accounts are channel-scoped. A channel may have several accounts for one provider.
Only configured, enabled accounts receive data; compiling a plugin alone sends nothing.
Catalogue deliveries contain public product data. Shopper events and their consent rules
continue through [event destinations](tracking.md), and are never sent to catalogue accounts.
Signals is optional for both.

## Accounts and scheduling

Use `skelet cli tracking kinds` to see fields required by installed adapters. Until the
dashboard destination pages land, configure accounts through `tracking add`. Nonsecret
fields use repeatable `--set key=value`; the credential is read with `--secret-stdin`
and encrypted using `CONFIG_ENCRYPTION_KEY`. Stdin preserves exact bytes, including
multiline keys, up to 64 KiB. Use `printf` rather than `echo` for passwords.
See [OpenAI SFTP](#openai-sftp) for SFTP, or add an HTTP receiver:

```bash
printf '%s' "$CATALOGUE_SIGNING_SECRET" | skelet cli tracking add \
  --channel <channel-id> --kind catalog_webhook --name 'Warehouse catalogue' \
  --secret-stdin --set endpoint=https://integrations.example.com/catalogue
skelet cli tracking catalog-send <destination-id>
skelet cli tracking deliveries <destination-id>
```

Core registers `<plugin-slug>.product-feed.<kind>` for each adapter, daily by default.
The scheduled job queues active channels' enabled accounts. Its history reports queueing;
inspect destination deliveries for rendering and upload results. Repeated requests for
an account coalesce while one delivery is pending, including during an active upload.

A separate worker renders and sends at most two catalogues concurrently per process,
with a 30-minute timeout and 31-minute claim lease. Event deliveries have their own worker
and short timeout. Transient failures retry after 10 seconds, 1 minute, 5 minutes,
30 minutes and 2 hours; permanent errors become dead letters immediately. Use
`skelet cli tracking retry <delivery-id>` for a dead delivery. Each attempt rebuilds the
latest catalogue with current account settings, rather than replaying an old snapshot.
A process crash can repeat an upload, so receivers must accept full replacement snapshots.

Delivery payloads contain only an empty object; account credentials and product rows are
resolved at execution time. Stored errors redact account secrets. Account or plugin
disablement stops pending deliveries when they are next claimed.

## Rendering and downloads

The shared source pages published channel listings using keyset pagination. It supplies
localized text, effective prices formatted from integer minor units, stock, images, brands
and variant selections. Provider writers decide which items meet their requirements.
Nonpositive-price products and product types without a fixed purchasable price are excluded.

Each account's snapshot is fully rendered to a temporary disk file before it replaces
`feeds/<kind>/<channel-id>/<destination-id>.<extension>` in object storage. Failed renders
preserve the previous snapshot. No complete catalogue is held in memory. A completed render
is retained even when its subsequent upload fails.

`GET /feeds/<kind>.<extension>?destination=<destination-id>` downloads the latest render.
Pass the channel using `X-Channel-Id`. The destination must belong to that channel and
be enabled; inactive channels and disabled plugins return 404. The destination parameter
can be omitted when only one matching account is enabled. These downloads expose published
product data publicly and contain no account credentials or shopper data.

## Signed HTTP receiver

`feed-webhook` POSTs UTF-8 JSONL, one normalized `sdk.ProductFeedItem` per line, with
`Content-Type: application/x-ndjson`. Availability includes unavailable products so a
receiver can update stock. Treat a successful request, including an empty body, as a full
replacement of that channel's previous catalogue.

| Header | Meaning |
|---|---|
| `X-Skelet-Channel-Id` | Channel UUID |
| `X-Skelet-Timestamp` | Unix seconds |
| `X-Skelet-Signature` | `sha256=` followed by the hex HMAC-SHA256 signature |

Compute HMAC using the account secret over the timestamp, a literal `.` and the exact raw
request body bytes. Compare in constant time and reject timestamps outside your accepted
clock skew (for example five minutes). Bind each signing secret to its expected channel;
the channel header is routing metadata, not part of the signature. Return 2xx only after
accepting the complete snapshot. Most 4xx responses are permanent failures; 408, 429 and
5xx retry. Redirects are not followed.

## Adding an adapter

Use `p.RegisterProductFeed(sdk.ProductFeed{...})` from the public SDK. Declare a unique
`Kind`, display `Label`, file `Extension`, MIME `ContentType` and account `Fields`.
At most one field may be marked `Secret`; use it for a token, signing key, password or
private key. `NewWriter(io.Writer, DestinationConfig)` returns a fresh writer with
`WriteItem(ProductFeedItem) (bool, error)` and `Close() error`. This lets the plugin own
CSV headers, JSON delimiters, compression and item eligibility. Return false to skip a
row; any write or close error fails the render.

`Send(context.Context, ProductFeedClient, DestinationConfig, ProductFeedFile)` uploads the
completed snapshot. `ProductFeedFile.Open` is repeatable and streams the file; its size,
name and content type are available too. Use the supplied client's `Do` for HTTP or
`UploadSFTP` for password/private-key SFTP. Both guard the actual resolved address against
private and metadata targets; development permits local test servers. SFTP requires a
pinned SHA256 server fingerprint and stages uploads before renaming the complete file.
Return `sdk.Permanent(err)` for configuration errors that a retry cannot fix.

Enable the plugin in `plugins/plugins.json` and run `skelet gen`. Core discovers its
account kind (`catalog_<kind>`), download route and scheduled job from the registry.
The OpenAI and webhook adapters are working examples.

## OpenAI SFTP

The `feed-openai` adapter writes compressed JSONL and uploads full snapshots to the
configured OpenAI SFTP account. It works independently of the `tracking-openai`
conversions plugin.

### Configure an account

Keep `feed-openai` enabled in `apps/backend/core/plugins/plugins.json`. Run `skelet gen`
and rebuild/restart core after changing the compiled plugin set. The channel must be active
and have a public storefront URL. Creating an enabled destination opts that channel in;
there is no separate `feed_enabled` setting.

Obtain merchant access, the SFTP address and credentials through OpenAI onboarding. Verify
the server's SHA256 host-key fingerprint through a trusted source before saving it.
Credentials are encrypted with `CONFIG_ENCRYPTION_KEY`, using the same account storage as
tracking destinations. Until the dashboard destination pages land, use the CLI:

```bash
printf '%s' "$SFTP_PASSWORD" | skelet cli tracking add \
  --channel <channel-id> --kind catalog_openai --name 'OpenAI catalogue' \
  --secret-stdin --set address=<sftp-host>:22 --set username=<sftp-user> \
  --set host_key=SHA256:<verified-fingerprint>
```

For SSH-key authentication, pipe the unencrypted private key file into the same command
and add `--set auth_method=private_key`. Pass `--set filename=catalog.jsonl.gz` to change
the stable remote filename. It must be a filename in the SFTP root, without directories.
Use a distinct filename for each account/channel sharing the same SFTP login.

The `feed-openai.product-feed.openai` scheduled job queues enabled accounts daily. Change
its schedule or run it from the scheduled-jobs page, or queue one account immediately:

```bash
skelet cli tracking catalog-send <destination-id>
skelet cli tracking deliveries <destination-id>
```

Uploads retry independently of the scheduler. A successful delivery means the file was
uploaded; OpenAI ingestion happens separately. See the [SFTP setup documentation](https://developers.openai.com/ads/product-feeds).

### What is exported

The source is the same paged reader used by the Google Merchant feed: published channel
listings, effective channel prices and special-price windows, default-locale text,
canonical paths, ready images, and salable stock including kit components. Configurable
and grouped parents, configurable children whose parent is unpublished, bundles with a
shopper-dependent price, and products marked noindex are excluded.

The OpenAI adapter excludes out-of-stock and nonpositive-price items. Backorderable items
remain eligible. Rows missing a title, description, brand, seller name, or absolute
product/image URL are skipped. Populate the product's `brand` attribute and content;
selectable brands are supported too.

Each row uses the SKU as `item_id`, the channel name as `seller_name`, and integer-derived
money in the currency's ISO minor units. Variants share their parent SKU as `group_id` and
carry selected attributes in `variant_dict`; list-mode variants use their SKU selection.
Search eligibility is on. Ads and Instant Checkout eligibility are explicitly off.
The adapter follows the [stable file-upload product schema](https://developers.openai.com/commerce/specs/file-upload/products).

### Inspect a snapshot

The last fully rendered snapshot is available from the **backend**:

```bash
curl --fail -H 'X-Channel-Id: <channel-id>' \
  'https://<backend-host>/feeds/openai.jsonl.gz?destination=<destination-id>' \
  -o openai.jsonl.gz
```

The destination parameter is optional when the channel has exactly one enabled OpenAI
catalogue account. Downloads return 404 before the first render, for another channel's
account, or when the account, channel or plugin is disabled. A render failure preserves
the previous local snapshot; an upload failure leaves the new snapshot available for
inspection. A successful empty render replaces the local file too.

OpenAI expects full snapshots at least daily using a stable filename. Products omitted
from a snapshot may remain in its retained catalogue for up to 14 days; omission does not
guarantee immediate removal. See [snapshot delivery and removal behavior](https://developers.openai.com/commerce/specs/file-upload/overview).

Local SSH/SFTP tests cover authentication, host-key verification and safe replacement.
Live OpenAI ingestion requires merchant credentials and has not been tested here.
