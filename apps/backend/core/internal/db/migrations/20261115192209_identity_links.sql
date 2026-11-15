-- migrate:up
CREATE TABLE identity_links (
    id         uuid        PRIMARY KEY DEFAULT uuidv7(),
    issuer     text        NOT NULL,
    subject    text        NOT NULL,
    kind       text        NOT NULL CHECK (kind IN ('admin', 'customer')),
    channel_id uuid        REFERENCES channels(id) ON DELETE CASCADE,
    user_id    uuid        NOT NULL,
    email      text        NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    last_used_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX identity_links_identity_idx ON identity_links (issuer, subject, kind, coalesce(channel_id, '00000000-0000-0000-0000-000000000000'));
CREATE INDEX identity_links_user_idx ON identity_links (kind, user_id);

-- migrate:down
DROP TABLE identity_links;
