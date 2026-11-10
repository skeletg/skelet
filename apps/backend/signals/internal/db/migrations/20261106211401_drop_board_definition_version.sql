-- migrate:up
-- A board report's definition is now typed on the wire and read as it's stored, so nothing reads its version number
ALTER TABLE board_reports DROP COLUMN definition_version;

-- migrate:down
ALTER TABLE board_reports ADD COLUMN definition_version integer NOT NULL DEFAULT 1;
