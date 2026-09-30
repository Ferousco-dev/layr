-- +goose Up
CREATE TABLE projects (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    name       text        NOT NULL CHECK (char_length(name) BETWEEN 1 AND 120),
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL
);

-- Serves the owner-scoped list ordered by recency with keyset pagination.
CREATE INDEX projects_user_recent_idx ON projects (user_id, updated_at DESC, id DESC);

-- +goose Down
DROP TABLE projects;
