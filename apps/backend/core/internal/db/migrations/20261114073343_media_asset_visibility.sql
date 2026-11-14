-- migrate:up
-- A private file is kept in the private store and read only through the API - never by an address anyone can open
ALTER TABLE media_assets ADD COLUMN visibility text NOT NULL DEFAULT 'public' CHECK (visibility IN ('public', 'private'));

-- migrate:down
ALTER TABLE media_assets DROP COLUMN visibility;
