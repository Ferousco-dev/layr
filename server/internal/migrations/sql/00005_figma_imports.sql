-- +goose Up
CREATE TABLE figma_imports (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id      uuid        NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    figma_file_key  text        NOT NULL,
    figma_node_id   text,
    figma_node_name text,
    figma_file_name text,
    figma_version   text,
    status          text        NOT NULL CHECK (status IN ('pending', 'processing', 'awaiting_selection', 'completed', 'failed')),
    error_code      text,
    candidates      jsonb,
    render_format   text,
    render_scale    numeric,
    created_at      timestamptz NOT NULL,
    updated_at      timestamptz NOT NULL,
    completed_at    timestamptz
);

-- Latest import and history for a project.
CREATE INDEX figma_imports_project_recent_idx ON figma_imports (project_id, created_at DESC, id DESC);

-- At most one import per project may be running; the database settles concurrent starts.
CREATE UNIQUE INDEX figma_imports_running_idx ON figma_imports (project_id) WHERE status IN ('pending', 'processing');

-- +goose Down
DROP TABLE figma_imports;
