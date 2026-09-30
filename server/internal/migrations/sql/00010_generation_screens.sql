-- +goose Up
ALTER TABLE generations ADD COLUMN screen_ids jsonb;

-- +goose Down
ALTER TABLE generations DROP COLUMN screen_ids;
