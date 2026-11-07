- **A fallback model for when the AI provider fails.** **Settings, AI, LLM provider** takes a fallback model
  (`ai/llm/fallback_model`) from the same provider. Every AI call - the shopping assistant, the search overview, admin
  chat and the rest - tries its model once more after a short pause on overload, rate limits, provider errors or a
  dropped connection, then answers on the fallback; a model that fails three calls in a row rests 30 seconds with its
  calls going straight to the fallback. A request the provider turns down is never retried elsewhere, and no retry
  starts without time to finish. Langfuse shows which model answered, and `ai.model.failovers` counts the switches.
