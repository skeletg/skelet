-- Segment rules name a customer group by slug, and the suite has no groups table to turn one into an
-- id - so the roster carries the slug beside the id, filled by core on every push
ALTER TABLE customer_roster ADD COLUMN customer_group_slug text NOT NULL DEFAULT '';
