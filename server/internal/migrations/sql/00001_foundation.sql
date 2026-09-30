-- +goose Up
-- The namespace marker proves migration wiring without inventing domain tables.
CREATE SCHEMA IF NOT EXISTS layr_foundation;

-- +goose Down
DROP SCHEMA IF EXISTS layr_foundation;
