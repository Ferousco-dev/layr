-- +goose Up
CREATE TABLE ai_credentials (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id        uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    provider       text        NOT NULL CHECK (provider IN ('anthropic', 'openai', 'xai')),
    key_ciphertext bytea       NOT NULL,
    key_hint       text        NOT NULL,
    created_at     timestamptz NOT NULL,
    updated_at     timestamptz NOT NULL,
    UNIQUE (user_id, provider)
);

-- +goose Down
DROP TABLE ai_credentials;
