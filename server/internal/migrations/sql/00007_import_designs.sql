-- +goose Up
ALTER TABLE figma_imports
    ADD COLUMN node_ids jsonb,
    ADD COLUMN screen_count integer,
    ADD COLUMN design_node_count integer,
    ADD COLUMN design_warning_count integer;

-- +goose Down
ALTER TABLE figma_imports
    DROP COLUMN node_ids,
    DROP COLUMN screen_count,
    DROP COLUMN design_node_count,
    DROP COLUMN design_warning_count;
