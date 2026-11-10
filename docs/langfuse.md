# Langfuse

[Langfuse](https://langfuse.com) shows what your AI calls did: each turn as a trace you can open, every model call in it
with its tokens and timing, and every tool it ran. Connecting it is optional, and nothing is sent until you do.

## Turning it on

Under **Settings, AI, Langfuse**:

- **Send AI calls to Langfuse** - on or off. Off by default.
- **Langfuse address** - `https://cloud.langfuse.com` for the EU cloud, `https://us.cloud.langfuse.com` for the US
  one, or your own Langfuse's address if you host it. Calls go through the same guard as webhooks, so a Langfuse on a
  private address (`10.x`, `192.168.x`, a container name) is only reached with `ALLOW_PRIVATE_WEBHOOK_TARGETS=true`
  outside development. Moving the address clears the secret key - enter it again for the new one.
- **Public key** and **Secret key** - from the Langfuse project's settings. The secret key is stored encrypted, like
  the store's other secrets.

A change takes effect within half a minute, no restart needed. A missing key or an address that isn't a URL leaves it
off, and says so once in the log.

## What it sends

- **A storefront assistant turn** is a trace named *Shopping assistant*, with the store and how the turn went: whether
  it changed the cart, sent them to checkout, was cut short or went unanswered. The page the shopper was on goes along
  only for a conversation the store keeps.
- Inside it, **each model call** is a generation with the model, the tokens it used - cached reads and writes apart -
  and how long it took, and **each tool the model ran** is a tool observation.
- **An admin chat message** is a trace named *Admin chat*, the same way.
- **The store's other AI work** - SEO text, translations, alt text, subject lines - sends each call as a generation.
  **Embeddings** show up as an embedding observation with the model and the tokens inside the turn that searched,
  never as a trace of their own.

## Outcomes and versions

Every storefront assistant trace is scored with how the turn went, so you can filter and evaluate by results rather than
read every answer:

- `unanswered` - 1 when the store couldn't answer: nothing said and shown, cut short, every search empty, or the
  assistant saying its information didn't cover it. The same test as the conversations' unanswered filter.
- `filled_cart` - 1 when the turn put something in the shopper's cart. Taking something out doesn't count.
- `led_to_order` - added later, when an order is placed from a cart the turn filled while the assistant still holds the
  credit for it, with the order number as its comment. The cart remembers the trace of the last turn that changed it,
  and the order scores that one.

Scores are Langfuse booleans, sent in the background to its scores API with an id per trace and name, so a replayed
event doesn't count twice. They're numbers about the turn, so they go whether or not the conversation is kept.

Each trace, and every observation in it, also carries a **version**: `assistant-` and a short fingerprint of what the
model is given before the shopper says anything - the prompt, with the store's name, voice and instructions, and the
tools. Change the voice, the instructions or upgrade to a release with new tools and the version changes, so Langfuse's
version filter sets answers before the change beside the answers after it.

## What it doesn't send

What shoppers wrote and what the assistant answered goes only for a conversation the store keeps - the same consent as
its [transcript](storefront-mcp.md#reading-conversations): the shopper agreed to analytics, the store
has **Keep the shopping assistant's conversations** on, and Signals is running. Then the trace also carries the
conversation as its session and the shopper as `customer:<id>` or `visitor:<id>`, and each model call carries the
prompt it was given, each tool what it was asked and what it answered.

For every other turn Langfuse gets the numbers alone: no words, no session, no one it belongs to.

The admin chat's tools read the store's customer data, so its traces never carry words either. Nor do the other calls:
the text of an SEO suggestion or a translation stays in the store, and what's embedded is never sent.

## How it's sent

The traces go over OpenTelemetry to Langfuse's `/api/public/otel` endpoint, signed with the project's keys, on a
pipeline of their own - apart from the server's own tracing (`OTEL_*`), so Langfuse never sees the server's HTTP and
database spans, and your collector never sees what shoppers asked. They're sent in the background in batches: a slow
or unreachable Langfuse never holds up an answer, and what it misses is dropped, not retried for ever. What's waiting
is sent as the server stops.

Erasing a customer doesn't reach what was already sent to Langfuse. If you keep conversations and send them to
Langfuse, delete a customer's traces there too - by the `customer:<id>` user, or the sessions of their conversations.
