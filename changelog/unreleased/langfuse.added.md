- **AI calls can go to Langfuse.** Under Settings, AI, Langfuse, with a project's keys and the EU, US or a self-hosted
  address, each shopping assistant turn and admin chat message arrives as a trace: every model call with its model,
  tokens and timing, and every tool it ran. What a shopper wrote and what the assistant answered goes only for a
  conversation the store keeps - the transcript's consent - and every other turn as numbers alone. It runs on its own
  OpenTelemetry pipeline, apart from the server's tracing, and in the background, so Langfuse being slow or down never
  holds up an answer. Each assistant trace is scored `unanswered` and `filled_cart`, and `led_to_order` once an order
  is placed from the cart it filled, and carries a version - a fingerprint of the prompt, the store's voice and the
  tools - so a change can be compared with what came before. Off by default; see `docs/langfuse.md`.
