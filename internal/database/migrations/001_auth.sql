-- +goose Up
CREATE TABLE users (
    id uuid PRIMARY KEY,
    username text NOT NULL,
    normalized_username text NOT NULL UNIQUE,
    display_name text NOT NULL DEFAULT '',
    email text NOT NULL DEFAULT '',
    auth_source text NOT NULL CHECK (auth_source IN ('local', 'ldap')),
    password_hash text,
    is_admin boolean NOT NULL DEFAULT false,
    disabled_at timestamptz,
    last_login_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK ((auth_source = 'local' AND password_hash IS NOT NULL) OR auth_source = 'ldap')
);

CREATE TABLE sessions (
    token_hash bytea PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    csrf_token text NOT NULL,
    expires_at timestamptz NOT NULL,
    last_seen_at timestamptz NOT NULL DEFAULT now(),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX sessions_user_id_idx ON sessions(user_id);
CREATE INDEX sessions_expires_at_idx ON sessions(expires_at);

-- +goose Down
DROP TABLE sessions;
DROP TABLE users;

