-- migrate:up
-- Boards get an owner and start private. A board with no owner is a shop board: everyone on the shop with boards.view
-- sees it, as every board did before this - so the boards already here keep working with no data change
ALTER TABLE boards ADD COLUMN owner_admin_id uuid;
CREATE INDEX boards_owner ON boards (owner_admin_id) WHERE owner_admin_id IS NOT NULL;

-- Shared with one admin or with everyone holding a role, to view or to edit
CREATE TABLE board_shares (
    board_id   uuid NOT NULL REFERENCES boards (id) ON DELETE CASCADE,
    admin_id   uuid,
    role_id    uuid,
    level      text NOT NULL CHECK (level IN ('view', 'edit')),
    created_by uuid NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CHECK ((admin_id IS NULL) <> (role_id IS NULL)),
    UNIQUE NULLS NOT DISTINCT (board_id, admin_id, role_id)
);

-- An admin's own starred boards - nobody else sees them
CREATE TABLE board_favourites (
    admin_id   uuid NOT NULL,
    board_id   uuid NOT NULL REFERENCES boards (id) ON DELETE CASCADE,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    PRIMARY KEY (admin_id, board_id)
);

-- An admin's unsaved edits to a board, kept between visits until they save or discard them. base_updated_at is the
-- board as it was when the draft began, so a save over someone else's newer one is still refused
CREATE TABLE board_drafts (
    board_id        uuid NOT NULL REFERENCES boards (id) ON DELETE CASCADE,
    admin_id        uuid NOT NULL,
    payload         jsonb NOT NULL,
    base_updated_at timestamp with time zone NOT NULL,
    updated_at      timestamp with time zone DEFAULT now() NOT NULL,
    PRIMARY KEY (board_id, admin_id)
);
CREATE INDEX board_drafts_age ON board_drafts (updated_at);

-- migrate:down
DROP TABLE board_drafts;
DROP TABLE board_favourites;
DROP TABLE board_shares;
DROP INDEX boards_owner;
ALTER TABLE boards DROP COLUMN owner_admin_id;
