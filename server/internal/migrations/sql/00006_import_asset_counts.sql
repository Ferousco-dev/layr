-- +goose Up
ALTER TABLE figma_imports ADD COLUMN asset_count integer, ADD COLUMN warning_count integer;

-- +goose Down
ALTER TABLE figma_imports DROP COLUMN asset_count, DROP COLUMN warning_count;
