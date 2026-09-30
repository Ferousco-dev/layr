-- +goose Up
CREATE TABLE users (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    figma_user_id text        NOT NULL UNIQUE,
    email         text,
    display_name  text        NOT NULL,
    avatar_url    text,
    created_at    timestamptz NOT NULL,
    updated_at    timestamptz NOT NULL
);

CREATE TABLE figma_connections (
    id                       uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id                  uuid        NOT NULL UNIQUE REFERENCES users (id) ON DELETE CASCADE,
    figma_user_id            text        NOT NULL UNIQUE,
    access_token_ciphertext  bytea       NOT NULL,
    refresh_token_ciphertext bytea       NOT NULL,
    token_expires_at         timestamptz NOT NULL,
    scopes                   text        NOT NULL,
    status                   text        NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'reconnect_required')),
    created_at               timestamptz NOT NULL,
    updated_at               timestamptz NOT NULL
);

CREATE TABLE sessions (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    token_hash   bytea       NOT NULL UNIQUE,
    expires_at   timestamptz NOT NULL,
    created_at   timestamptz NOT NULL,
    last_seen_at timestamptz NOT NULL,
    revoked_at   timestamptz
);

CREATE INDEX sessions_user_id_idx ON sessions (user_id);

-- +goose Down
DROP TABLE sessions;
DROP TABLE figma_connections;
DROP TABLE users;
