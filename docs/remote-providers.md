# Building a remote provider

A **remote provider** is an HTTPS endpoint you own that Skelet calls mid-request to make a
decision: what shipping options to offer, how much tax is owed, whether an order looks
fraudulent. Where a webhook *observes* something that already happened, a provider
*implements* the answer - core waits for your response and uses it.

You can write one in any language. It is one POST endpoint that verifies a signature and
returns JSON. Nothing is installed into Skelet, and nothing needs rebuilding or
redeploying - a merchant registers the URL in the admin and it is live.

Not covered here: payment gateways. Those are deliberately *not* a provider kind, because
an authorization needs a client secret reaching the browser for 3DS, a vault token
contract, and verification of the gateway's own inbound webhooks. Proxying that would put
your endpoint in PCI scope. Gateways are first-party plugins instead.

## The five-minute version

```bash
# 1. Register the provider in the admin (Settings -> Remote providers) and copy the
#    rpsec_... secret - it is shown exactly once.
# 2. Stand up one POST endpoint that verifies the signature below and answers with the
#    kind's response shape, then check it against the contract before wiring it to
#    anything real.
./skelet provider --url http://localhost:9095/tax --secret rpsec_... --kind tax_provider
```

`./skelet provider` posts the generated example payload for the kind, then tries the
failure cases - forged signature, no signature, wrong key, an hour-old timestamp - and
tells you whether your endpoint handled each correctly. The request, the signature and
every kind's payload and response are specified in the rest of this page.

## The request

Core sends `POST` to your URL with:

| Header | Value |
|---|---|
| `Content-Type` | `application/json` |
| `User-Agent` | `RemoteProvider/1.0` |
| `X-Provider-Kind` | the kind, e.g. `tax_provider` |
| `X-Provider-Contract-Version` | the wire-contract version, e.g. `0.1.0` |
| `X-Provider-Slug` | the slug the merchant registered |
| `X-Provider-Timestamp` | unix seconds |
| `X-Provider-Signature-256` | `sha256=<hex>` |

The body is:

```json
{
  "kind": "tax_provider",
  "provider": "acme-tax",
  "contractVersion": "0.1.0",
  "timestamp": "2026-08-22T10:31:00Z",
  "config": { "apiKey": "..." },
  "payload": { }
}
```

`config` is the JSON blob the merchant saved on the provider row - credentials, tuning,
whatever your service needs - forwarded verbatim and omitted when empty. `payload` is
kind-specific and documented per kind below.

## Verifying the signature

```
signature = HMAC_SHA256(secret, X-Provider-Timestamp + "." + raw_request_body)
```

Hex-encode it and compare against the header value with the `sha256=` prefix stripped.

Three things to get right, in the order people get them wrong:

**Sign over the header timestamp, not the one in the body.** There are two timestamps and
they are not the same value. The body's `timestamp` is RFC3339 and is generated when core
builds the envelope; `X-Provider-Timestamp` is unix seconds and is generated later, just
before the request goes out. **The signature uses the header.** Signing over the body's
field produces a digest that never matches, and the symptom - every request 401s, including
obviously valid ones - looks like a wrong secret.

**Hash the raw bytes.** Verify before parsing, and hash exactly what arrived. Decoding and
re-encoding the JSON reorders keys and changes whitespace, so the digest will not match.

**Compare in constant time.** `hmac.Equal`, `crypto.timingSafeEqual`, `hmac.compare_digest`
- not `==`.

Core does not cap how old a timestamp may be, so **you** should. The reference
implementation rejects anything outside five minutes; without that, one captured request
stays replayable forever.

<details>
<summary>Verification in TypeScript, Python and PHP</summary>

```ts
import { createHmac, timingSafeEqual } from "node:crypto";

// rawBody must be the unparsed body - express: app.use(express.raw({type:"application/json"}))
export function verify(secret: string, headers: Record<string,string>, rawBody: Buffer): boolean {
  const ts = headers["x-provider-timestamp"];
  if (!ts || Math.abs(Date.now() / 1000 - Number(ts)) > 300) return false;
  const want = createHmac("sha256", secret).update(ts).update(".").update(rawBody).digest("hex");
  const got = (headers["x-provider-signature-256"] ?? "").replace(/^sha256=/, "");
  const a = Buffer.from(got), b = Buffer.from(want);
  return a.length === b.length && timingSafeEqual(a, b);
}
```

```python
import hmac, hashlib, time

def verify(secret: str, headers, raw_body: bytes) -> bool:
    ts = headers.get("X-Provider-Timestamp", "")
    if not ts or abs(time.time() - int(ts)) > 300:
        return False
    want = hmac.new(secret.encode(), ts.encode() + b"." + raw_body, hashlib.sha256).hexdigest()
    got = headers.get("X-Provider-Signature-256", "").removeprefix("sha256=")
    return hmac.compare_digest(got, want)
```

```php
function verify(string $secret, array $headers, string $rawBody): bool {
    $ts = $headers['X-Provider-Timestamp'] ?? '';
    if ($ts === '' || abs(time() - (int)$ts) > 300) return false;
    $want = hash_hmac('sha256', $ts . '.' . $rawBody, $secret);
    $got = preg_replace('/^sha256=/', '', $headers['X-Provider-Signature-256'] ?? '');
    return hash_equals($want, $got);
}
```

</details>

## The response

Return **HTTP 2xx** with a JSON body matching the kind's response schema. Anything else -
3xx, 4xx, 5xx, a non-JSON body, an empty body - is an invocation failure.

Three limits worth knowing before you hit them:

- **256 KiB.** Core reads your response through a limit reader and stops there. A larger
  body is silently truncated and then fails to parse, so an oversized response reports as a
  malformed one. Paginate or trim rather than returning everything.
- **Your timeout.** The merchant sets it per provider (default 5s, max 30s). Past it core
  abandons the call and applies the failure policy.
- **Null is not zero.** Several response fields are nullable *by design*, and null means
  "no answer" rather than a zero value - `taxCents`, `unitPriceCents`,
  `shippingOverrideCents`, `average`, `normalized`. Returning null where you mean zero gets
  read as a failure. Send the number.

## What happens when you fail

**Failure policy**, set per provider by the merchant:

- `fail_open` (default) - core falls back to its own behavior. Your tax provider being down
  means built-in tax rules apply.
- `fail_closed` - the operation aborts. Safer when a wrong answer is worse than no sale.

Some consumers pick per operation regardless of the setting. Shipping is the clearest
example: quoting fails open, so a dead carrier just drops out of the list, but pricing the
method the customer already chose fails closed rather than charging a guessed rate.

**Circuit breaker.** Five consecutive failures opens the circuit for that provider row;
calls short-circuit for 30 seconds, then one is allowed through to test recovery. The admin
shows breaker state, so a provider that looks "not called" may be tripped rather than
unused.

**Response cache.** If the merchant sets a cache TTL (max 1 hour), successful responses are
cached against a hash of **the payload alone** - the envelope timestamp is deliberately
excluded, so an identical payload will not reach you again until the TTL expires. Two
consequences: identical inputs must produce identical outputs, and a provider whose answer
depends on wall-clock time (a rate that changes hourly, live inventory) wants a short TTL or
none. Editing the provider's config or URL flushes its cache.

**Channel scope.** A provider can be bound to one channel or left global. A channel-bound
provider is only called for that channel's traffic.

## Contract versioning

Every request carries the wire-contract version, both as `X-Provider-Contract-Version` and
as `contractVersion` inside the signed body, so nothing between core and you can strip it.
The version names the shape of every payload in this document - it is a single number for
the whole contract, not one per kind.

The contract is pre-1.0 and versioned accordingly:

- a **minor** bump (`0.1.x` -> `0.2.0`) means something broke: a field was removed or
  renamed, a type changed, a kind was removed, or core started requiring something new in
  responses. Re-run `./skelet provider` against your endpoint before upgrading core.
- a **patch** bump means additive only: a new kind, or a new optional field. Nothing you
  already handle changed shape.

Two habits keep you safe across patch bumps: ignore unknown JSON fields (Go's default), and
never treat an absent optional field as an error. Log the version you receive - when a
payload surprises you, the first question is which contract it was built under, and core's
side of the guarantee is machine-enforced: the generated schemas cannot change without a
version bump.

## The kinds

Machine-readable schemas and a ready-to-post example for every operation live in
[`apps/backend/core/internal/remoteprovider/contract/`](../apps/backend/core/internal/remoteprovider/contract/)
as `<name>.schema.json` and `<name>.request.json`. They are generated from core's own
structs by `./skelet gen`, so they cannot drift from what core actually sends.

| Kind | What you decide |
|---|---|
| `shipping_carrier` | Rate a basket to a destination; the options the shopper picks from |
| `delivery_points` | Search and resolve PO boxes and lockers for a PO box delivery option; see [checkout.md](checkout.md) |
| `shipping_tracking` | Carrier tracking status for a shipment |
| `tax_provider` | Tax owed on an order |
| `address_validator` | Whether an address is valid, plus a normalized form |
| `promotion_condition` | Whether a promotion rule matches the cart |
| `promotion_action` | The discount a matched rule applies |
| `pricing` | Override a line's unit price |
| `currency_rate` | FX rates for a base currency |
| `fraud_risk` | Approve or reject an order |
| `returns_policy` | Approve or refuse a return |
| `inventory_availability` | Salable quantity per product (`-1` = unlimited) |
| `recommendation` | Related products for a product page |
| `reviews` | Review aggregates from an external platform |
| `search` | Free-text search **and** faceted browse - two operations, one kind |
| `customer_segments` | Segment slugs a customer belongs to |

Three notes that cost debugging time:

- **`search` is two operations.** A provider registered for `search` must implement both
  `search` (typeahead and `/search`) and `search.browse` (category and filter listings).
  Dispatch on the payload shape, or on `X-Provider-Kind` plus the presence of `filter`.
- **Addresses are not one shape.** `address_validator` uses `state`; the tax, fraud and
  shipping addresses use `region`. Same concept, different field name - do not share a
  struct across kinds without checking the schema.
- **`fraud_risk` approves by default.** Only the exact string `"reject"` in `decision`
  blocks an order. Any other value, including a typo or an empty string, approves it.

## Registering

Admin, under Remote providers - needs the `remote_providers.manage` permission. You set the
kind, a slug, the target URL, the config blob, timeout, cache TTL, failure policy, and
optionally a channel. The `rpsec_` signing secret is displayed once on creation; rotating it
issues a new one and invalidates the old.

Then, before you point production at it:

```bash
./skelet provider --url https://your-provider.example.com/hook \
  --secret rpsec_... --kind tax_provider --timeout-ms 5000
```

Pass `--payload FILE` to test with your own request body instead of the generated example,
and `--config FILE` to include the config blob your endpoint expects.
