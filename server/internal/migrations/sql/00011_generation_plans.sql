-- +goose Up
CREATE TABLE generation_plans (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id       uuid        NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    import_id        uuid        NOT NULL,
    design_version   text        NOT NULL CHECK (design_version <> ''),
    fingerprint      text        NOT NULL CHECK (fingerprint ~ '^[0-9a-f]{64}$'),
    selection_mode   text        NOT NULL CHECK (selection_mode IN ('one', 'selected', 'flow', 'all')),
    target_framework text        NOT NULL,
    target_language  text        NOT NULL,
    status           text        NOT NULL CHECK (status IN ('planned', 'invalid')),
    schema_version   integer     NOT NULL,
    plan             jsonb       NOT NULL,
    created_at       timestamptz NOT NULL,
    updated_at       timestamptz NOT NULL
);

CREATE INDEX generation_plans_project_recent_idx ON generation_plans (project_id, created_at DESC, id DESC);
CREATE INDEX generation_plans_fingerprint_idx ON generation_plans (project_id, fingerprint);

-- +goose Down
DROP TABLE generation_plans;
