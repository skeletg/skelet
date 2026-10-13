-- The primary key (page_id, locale, variant_key) already leads with page_id, so this index only cost writes
DROP INDEX IF EXISTS idx_cms_page_variant_content_page;
