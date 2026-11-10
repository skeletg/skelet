-- migrate:up
-- An anchor category's page lists the products of every published category beneath it as well as its own
ALTER TABLE catalog_categories ADD COLUMN is_anchor boolean NOT NULL DEFAULT true;

-- migrate:down
ALTER TABLE catalog_categories DROP COLUMN is_anchor;
