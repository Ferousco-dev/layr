-- +goose Up
ALTER TABLE projects ADD COLUMN deleted_at timestamptz;

DROP INDEX projects_user_recent_idx;
CREATE INDEX projects_user_recent_idx ON projects (user_id, updated_at DESC, id DESC) WHERE deleted_at IS NULL;

-- Lets the lazy purge find expired deletions without scanning live projects.
CREATE INDEX projects_deleted_idx ON projects (deleted_at) WHERE deleted_at IS NOT NULL;

-- +goose Down
DROP INDEX projects_deleted_idx;
DROP INDEX projects_user_recent_idx;
DELETE FROM projects WHERE deleted_at IS NOT NULL;
ALTER TABLE projects DROP COLUMN deleted_at;
CREATE INDEX projects_user_recent_idx ON projects (user_id, updated_at DESC, id DESC);
