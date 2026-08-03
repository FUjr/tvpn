-- +goose Up
CREATE TABLE program_tokens (
    id uuid PRIMARY KEY,
    token_hash bytea NOT NULL UNIQUE,
    token_prefix text NOT NULL,
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name text NOT NULL,
    scopes text[] NOT NULL,
    expires_at timestamptz NOT NULL,
    last_used_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    revoked_at timestamptz,
    CHECK (length(name) BETWEEN 1 AND 100),
    CHECK (cardinality(scopes) > 0)
);
CREATE INDEX program_tokens_user_id_idx ON program_tokens(user_id);
CREATE INDEX program_tokens_expires_at_idx ON program_tokens(expires_at);

-- +goose Down
DROP TABLE program_tokens;
