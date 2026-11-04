-- migrate:up
-- Boards: a shop's shared dashboards of saved reports. A report's definition is JSON with its own version number, so
-- the shape can grow without a migration and an old definition can still be read
CREATE TABLE boards (
    id         uuid DEFAULT uuidv7() NOT NULL PRIMARY KEY,
    channel_id uuid NOT NULL,
    name       text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);
CREATE INDEX boards_channel ON boards (channel_id, name);

CREATE TABLE board_reports (
    id                 uuid DEFAULT uuidv7() NOT NULL PRIMARY KEY,
    board_id           uuid NOT NULL REFERENCES boards (id) ON DELETE CASCADE,
    position           integer NOT NULL,
    title              text NOT NULL,
    definition         jsonb NOT NULL,
    definition_version integer NOT NULL DEFAULT 1,
    UNIQUE (board_id, position)
);

-- migrate:down
DROP TABLE board_reports;
DROP TABLE boards;
