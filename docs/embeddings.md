# Embeddings

The store keeps a vector for every product and published page, per language, so it can be searched by meaning - a
shopper asking for "something warm for winter hikes" can find a jacket that never uses those words. The shopping
assistant's searches read them, and so do shoppers' own agents over the storefront's MCP endpoint, since they share
the tools. The storefront's search page reads them too, for searches written as sentences and ones whose words find
nothing.

## Turning it on

Embeddings need their own provider, since Anthropic doesn't make them. Under **Settings, AI, Embeddings**:

- **Model** - blank keeps it off. It has to give vectors of 1024 numbers: OpenAI's `text-embedding-3-small` and
  `-large` are asked for that size, Voyage's `voyage-3.5` and Ollama's `mxbai-embed-large` give it anyway. A model
  that answers in another size is refused, and **System health** says so.
- **Base URL** - blank is OpenAI. Anything that answers OpenAI's `POST /embeddings` works: Voyage, Azure, Ollama,
  LM Studio. Each request also says whether it's a search or a document, which some models find more with. A
  server that refuses either field, the size or that one, is asked without it from then on.
- **API key** - blank for a local server. For OpenAI's own endpoint, `OPENAI_API_KEY` stands in.

`AI_ENABLED=false` turns embeddings off along with the rest of the AI features.

## What gets embedded

- **Products** a shopper can find - listed in at least one store, and not a variant sold through its parent. One
  passage per language: the name, the product type, its categories, its **public** attribute values, and the
  descriptions. Private attributes never reach the provider. Prices and stock stay out, since they change all the
  time and aren't what a product is. An attribute's **Search by meaning** switch (on by default) leaves a public one
  out too - care instructions or a country of origin say little about what a product is, and only pull every
  product's vector toward the same words. Flipping it re-embeds the products that carry the attribute.
- **Published CMS pages**, split into passages of a few paragraphs at their headings, each led by the page title and
  its heading - so "do you deliver to Norway?" lands on the paragraph that answers it, not a whole shipping page.
  Accordion and tab items, the usual home of an FAQ, are passages of their own.

Which store shows what is checked when searching, the same way listings are, so one set of vectors serves every
store.

## How search uses them

- **`search_products`** turns the shopper's words into a vector and finds the store's nearest listed products,
  through the HNSW index - a few milliseconds over a large catalog. They're ranked together with the products the
  words match by reciprocal rank fusion: one high in either list comes high, one in both higher still. Products the
  words miss entirely still come back, under a line saying they're close in meaning rather than matching. Only the
  first page of a plain search mixes them in - with a filter, a category, a price range or a sort, the model asked for
  exactly that list, and gets it.
- **`search_store_pages`** finds the passages nearest the question, among the pages the store publishes, and ranks
  their pages with the ones the words find. Each page comes back with the passages that came close, so the assistant
  answers from the paragraph that says it, rather than the start of the page.
- Nearest neighbours always answer, relevant or not, so a hit counts only within a margin of the best one, sized to
  how far that best one is - how far is far depends on the model, and the store's own best match is a fair yardstick
  for any model. A best match that's very close or very far is held to sensible limits, so the margin never shrinks to
  nothing or swallows the catalog.
- The store's **Search by meaning** settings (below) govern the assistant too: **Off** keeps its searches on the
  words, and how far meaning reaches sets its margin.
- The shopper's language is searched first, then the store's own, for anything not written in theirs.
- A question's vector is kept in memory for ten minutes, so a follow-up or a retried turn doesn't call the provider
  again, and two searches asking the same question at once share one call. When the provider fails, search answers
  on the words alone, logs it and leaves the provider be for thirty seconds rather than wait on it with every
  shopper.

The **Assistant** tab of **Agentic Commerce, Hybrid search** runs both searches for a question you type, on the
selected store and in the language you pick, with the page's choices as they stand. It shows each list on its own -
what the words matched, the nearest by meaning with their distance and whether they made the margin - then what
`search_products` hands the model, in its order, and both tools' answers word for word. It's the quickest way to see
whether a missed question is the catalog, the words or the model.

### Similar products

**`find_similar_products`** answers "more like this" for the assistant and shoppers' agents: the store's listed products
nearest in meaning to one to five products together (their vectors averaged), closest first, the products themselves
left out. A variant stands for the product it's sold through, and an exact product name works as well as an id, so the
assistant needn't search for it first.

An attribute's **Must match for similar products** switch, on its page under Catalog, holds what's found to the
products' own values on it: switch it on for Gender and a men's jacket never answers "more like this dress", for Brand
and the answer stays in the brand. With several products, a value any of them has counts, so "like these two" in Black
and Navy allows either. A product without a value on a switched-on attribute the given ones have doesn't match. Values
picked from a list match by the value, typed ones by their text, case and spacing aside, and a product's variants'
values count as its own - colour usually lives there. It's off for every attribute until you switch it on.

A value that stands for all the others - Unisex for Gender, One size for Size, Multicolour for Colour - gets **Matches
every value** in its row on the attribute page. It then matches any value of that attribute, either way round: a
unisex scarf answers "more like this dress", and a women's dress answers "more like this unisex scarf". The product
still needs some value on the attribute, and every other switched-on attribute still applies.

It answers from the vectors already stored, so it costs no call to the provider. Products sit far closer to each other
than to a question, so it doesn't use search's reach - the matching attributes and the number asked for keep it on
target. **Off** under Hybrid search turns it off with the rest of search by meaning.

### The storefront's own search

The search page and search box (`browseProducts`) use meaning too. **Agentic Commerce, Hybrid search** holds its two
settings - per store, or for every store - and a box that runs a search with them, saved or not. Its **Search page**
tab shows what the shopper sees and how each half got there, its **Assistant** tab what the assistant finds for the
same words, so a change of reach shows on both. The same settings sit under **Settings, Search, Product Search**.

**Use meaning:**

- **For sentences, and when words find nothing** (the default). A query that reads like a sentence - a question, a few
  words joined by one like "for", "with" or "I need", or four or more words - lists the products closest in meaning,
  and the words only help order them. A sentence's words match all sorts: "high quality shirt" finds High-Rise jeans
  on "high", and here they can't. A short keyword search stays on its words, so "linen shirt", a brand or a SKU
  behaves as it always has; when its words match nothing, the few products closest in meaning are listed instead of
  an empty page. Codes don't count as words, so "air max 90" or a part number is never read as a sentence.
- **Every search** ranks words and meaning together, the way the assistant does - for any query whose words find 48
  products or fewer. Past that the words' own listing stands, since a blend lists at most 48 and would cut it short.
- **Off** keeps it on the words.

**How far meaning reaches** sets how close to the best match a product must be to be listed. Strict lists only near
twins of what was asked for, balanced (the default) reasonably close ones, broad reaches further for related products.
A keyword search that matched nothing reaches about a third as far, since its words may name something the store
doesn't sell. However far it reaches, meaning lists at most 48 products.

What meaning finds is handed to the engine as the products to list, so the shopper's filters, the facet counts,
sorting and paging work as on any listing - only "relevance" is meaning's order, with the store's merchandising boosts
and its out-of-stock-last setting applied as on any search. Searching inside a category or collection reads the 200
products nearest in meaning and keeps the ones in it; when none are, the words answer. A store's searches ask the
provider at most 1200 new questions a minute across every server - a question already asked is free - and each waits
a second and a half for it, so a burst of shoppers or a slow provider gets the words' listing rather than a stall. Each shopper may also send it only `search/products/meaning_per_minute`
new questions a minute (60 by default, 0 for no limit, a question already asked is free) - past that their searches
run on their words, so one visitor can't spend the store's share of the provider. It works on OpenSearch and on the
Postgres tier; a remote search provider answers keyword searches as before, but a meaning listing goes to the native
engine, since the provider contract has no way to carry a list of products. With embeddings off, or the provider
down, every search runs on its words. The assistant blends on its own, so the searches it runs read the words alone.

### AI overview

A search meaning answers can open with an overview: two or three sentences on what the products listed offer for what
was asked, in the shopper's language, each product it names a link to its page, and a **Continue in chat** button that
opens the shopping assistant with the search as the shopper's message and the overview as its first answer, so "in
white?" or "under $100?" carries on from there.

- **Where:** only the first page of a search with no filters, and only when meaning answered it - a sentence, or
  keywords that matched nothing. A keyword search its words answered never gets one.
- **Who:** only a search made on the store - typed into its search box or followed from one of its pages. A search page
  opened straight from a link, a bookmark or another site lists its results without one, and so does a crawler, so a
  scraper walking search URLs never starts the AI. The storefront tells by the browser's `Sec-Fetch-Site` header, or
  the referrer where a browser doesn't send it.
- **What it reads:** the first six products listed, through what was embedded for each (the same public words, never a
  private attribute), plus the nearest passages from the store's pages. Prices, stock and delivery times stay out - the
  page shows those itself. It writes in the assistant's voice from **Assistant voice**.
- **Speed:** the results render first and the overview streams in after, from the quick model under **Settings, AI,
  LLM provider** when one is set, else the main one. The storefront gives up after a few seconds and shows nothing,
  while the backend finishes it anyway, so the next shopper gets it from the cache.
- **Cost:** each one is cached for a day per store, language, search and products, and a change to the assistant's
  voice or to a listed product's words writes it afresh. A new one counts against the shopper's pace and its own daily allowance beside the
  assistant's chats (the same number, counted apart), and its tokens count toward the assistant's daily token cap.
  **Agentic Commerce, Overview** shows how many were written.
- **Langfuse:** each new one is a *Search overview* trace with the model and tokens - numbers only, since a search is the
  shopper's own words.

It's off by default. **Agentic Commerce, Hybrid search** turns it on per store (it needs the shopping assistant on)
and its Search page tab previews it for a search before you do.

A keyword search whose words matched nothing, listed by meaning instead, says so above the results - "Nothing matched
"breezy" word for word, so these are the closest to what you're after" - so loose matches read as intended rather than
wrong. It's still a gap in how the catalog is worded, so the storefront tells Lens too, and **Lens, Search** lists
those terms apart from the searches that found nothing at all.

## How it keeps up

Saving a product or a page, changing its listing or an attribute it carries, and publishing or unpublishing a page
each put it in a queue. The `embeddings` job runs every minute, reads the queue, works out the text again, and sends
the provider only the passages whose words changed - an edit to price or stock costs nothing. Something unpublished
or deleted loses its passages.

With the queue empty, the job picks up whatever has never been embedded with the current model: the first run on
an existing catalog, a new model, or something saved while the job was down. Changing the model embeds everything
again, and the old model's vectors go as the new ones arrive. Deleting a language drops its passages with it.

**System health**, under Search, shows the model and how many items are waiting, and warns when the provider
fails or the queue stops moving.

## Cost

One passage is a few hundred tokens, and embedding models cost cents per million tokens, so a catalog of 100,000
products costs well under a dollar to embed the first time. After that, only edits cost anything.

## Postgres

The vectors live in Postgres with the `pgvector` extension, which the commerce database now needs. The bundled
database runs the `pgvector/pgvector` image, and every major managed Postgres - RDS, Cloud SQL, Azure, DigitalOcean,
Neon, Supabase - has the extension: the migration creates it, or, where the app's database user may not create
extensions, run `CREATE EXTENSION vector;` once as the admin user first. Moving an existing volume from the old
Alpine image is covered in [deploying.md](deploying.md#moving-to-the-pgvector-image).
