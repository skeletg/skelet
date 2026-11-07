-- migrate:up
CREATE EXTENSION IF NOT EXISTS vector;

-- One row per passage: a product is a single passage, a page one per section. The model is part of the key, so switching
-- models fills a fresh set rather than mixing vectors two models made
CREATE TABLE public.embeddings (
    source text NOT NULL,
    source_id uuid NOT NULL,
    -- Built from content, so it goes with its locale rather than holding the locale in place
    locale text NOT NULL REFERENCES public.locales (code) ON DELETE CASCADE,
    model text NOT NULL,
    chunk smallint NOT NULL,
    content text NOT NULL,
    content_hash bytea NOT NULL,
    embedding vector(1024) NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    PRIMARY KEY (source, source_id, locale, model, chunk),
    CONSTRAINT embeddings_source_check CHECK (source IN ('product', 'cms_page'))
);
-- Searches by meaning read products by vector, and a store's pages are few enough to read without one
CREATE INDEX idx_embeddings_product_vector ON public.embeddings USING hnsw (embedding vector_cosine_ops) WHERE source = 'product';
CREATE INDEX idx_embeddings_model ON public.embeddings (model);

-- What changed since the last pass - a product saved twice before the job runs is still one row. One the provider keeps
-- refusing waits out a growing pause instead of holding the head of the queue
CREATE TABLE public.embedding_queue (
    source text NOT NULL,
    source_id uuid NOT NULL,
    queued_at timestamp with time zone DEFAULT now() NOT NULL,
    attempts integer DEFAULT 0 NOT NULL,
    PRIMARY KEY (source, source_id),
    CONSTRAINT embedding_queue_source_check CHECK (source IN ('product', 'cms_page'))
);

-- Whether a public attribute's values go into the text embedded for search by meaning - off keeps a fact like care
-- instructions on the storefront without it pulling every product's vector toward laundry - and whether a product found
-- similar to others must share a value with them on it, so a men's jacket doesn't answer "more like this dress"
ALTER TABLE public.catalog_attributes
    ADD COLUMN embedded boolean DEFAULT true NOT NULL,
    ADD COLUMN similar_match boolean DEFAULT false NOT NULL;

-- A value that stands for all the others - Unisex, One size, Multicolour - matches every value of its attribute when
-- similar products are held to it
ALTER TABLE public.catalog_attribute_values ADD COLUMN matches_any boolean DEFAULT false NOT NULL;

-- migrate:down
ALTER TABLE public.catalog_attribute_values DROP COLUMN matches_any;
ALTER TABLE public.catalog_attributes DROP COLUMN similar_match, DROP COLUMN embedded;
DROP TABLE public.embedding_queue;
DROP TABLE public.embeddings;
DROP EXTENSION IF EXISTS vector;
