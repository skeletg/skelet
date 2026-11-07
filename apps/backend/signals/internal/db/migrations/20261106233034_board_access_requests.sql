-- migrate:up
-- Someone who opened a board they can't see asked its owner for access. One open request per admin per board; a later
-- one replaces it. It goes when they're shared the board or the owner dismisses it
CREATE TABLE board_access_requests (
    board_id   uuid NOT NULL REFERENCES boards (id) ON DELETE CASCADE,
    admin_id   uuid NOT NULL,
    level      text NOT NULL CHECK (level IN ('view', 'edit')),
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    PRIMARY KEY (board_id, admin_id)
);

-- migrate:down
DROP TABLE board_access_requests;
