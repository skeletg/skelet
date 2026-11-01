# OpenAI product feed

`feed-openai` is the OpenAI adapter for [catalogue delivery](catalogue-delivery.md).
It writes compressed JSONL and uploads full snapshots to the configured OpenAI SFTP account.
It works independently of Signals and the `tracking-openai` conversions plugin.

## Configure an account

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

## What is exported

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

## Inspect a snapshot

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
