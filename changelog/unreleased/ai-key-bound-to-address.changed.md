- **An AI key only goes to the address it was entered for.** Every call checks it, so moving the LLM, embeddings or
  Langfuse address never sends the old key there, even for a moment. A key from the environment (`AI_API_KEY`,
  `ANTHROPIC_API_KEY`, `OPENAI_API_KEY`) is only sent to the provider's own address - a custom `ai/llm/base_url` needs
  its key entered in the settings. Saving a setting without changing it no longer counts as a change.
