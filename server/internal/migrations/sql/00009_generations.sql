-- +goose Up
CREATE TABLE generations (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id   uuid        NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    provider     text        NOT NULL CHECK (provider IN ('anthropic', 'openai', 'xai')),
    status       text        NOT NULL CHECK (status IN ('running', 'completed', 'failed')),
    steps        jsonb       NOT NULL DEFAULT '[]',
    error_code   text,
    created_at   timestamptz NOT NULL,
    updated_at   timestamptz NOT NULL,
    completed_at timestamptz
);

CREATE INDEX generations_project_recent_idx ON generations (project_id, created_at DESC, id DESC);
CREATE UNIQUE INDEX generations_running_idx ON generations (project_id) WHERE status = 'running';

-- +goose Down
DROP TABLE generations;
