-- migrate:up
-- A report's width on its board - layout, so it sits beside the title rather than in the definition the engine reads
ALTER TABLE board_reports ADD COLUMN size text NOT NULL DEFAULT '' CHECK (size IN ('', 'small', 'medium', 'full'));

-- migrate:down
ALTER TABLE board_reports DROP COLUMN size;
