-- migrate:up
ALTER TABLE countries ADD COLUMN invoice_note text NOT NULL DEFAULT '';

-- migrate:down
ALTER TABLE countries DROP COLUMN invoice_note;
