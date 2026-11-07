- **Embeddings for products and pages.** With a model set under **Settings, AI, Embeddings** - OpenAI, Voyage,
  Azure, Ollama or anything else that answers OpenAI's `/embeddings` - the store keeps a vector for every listed
  product and published CMS page, per language, with pages split into passages at their headings. Saves queue
  what changed, and an `embeddings` job every minute embeds only passages whose words changed, then works through
  anything never embedded. **System health** shows its progress. See `docs/embeddings.md`.
- **The shopping assistant searches by meaning.** `search_products` ranks the store's products closest in meaning
  with the ones its words match, so "something warm for winter hikes" finds the down jacket, and
  `search_store_pages` answers with the passage that says it. Shoppers' own agents get the same over the
  storefront MCP endpoint. Filtered, sorted and later pages stay on the words.
