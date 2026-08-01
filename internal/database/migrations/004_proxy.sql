-- +goose Up
CREATE TABLE proxy_contexts (
    id uuid PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    current_url text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    last_active_at timestamptz NOT NULL DEFAULT now(),
    closed_at timestamptz
);
CREATE TABLE proxy_routes (
    id uuid PRIMARY KEY,
    context_id uuid NOT NULL REFERENCES proxy_contexts(id) ON DELETE CASCADE,
    upstream_scheme text NOT NULL CHECK (upstream_scheme IN ('http','https')),
    upstream_host text NOT NULL,
    upstream_port integer NOT NULL CHECK (upstream_port BETWEEN 1 AND 65535),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (context_id, upstream_scheme, upstream_host, upstream_port)
);
CREATE TABLE proxy_tickets (
    token_hash bytea PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    context_id uuid NOT NULL REFERENCES proxy_contexts(id) ON DELETE CASCADE,
    expires_at timestamptz NOT NULL,
    used_at timestamptz
);
CREATE TABLE proxy_sessions (
    token_hash bytea PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE proxy_cookies (
    id uuid PRIMARY KEY,
    context_id uuid NOT NULL REFERENCES proxy_contexts(id) ON DELETE CASCADE,
    name text NOT NULL,
    domain text NOT NULL,
    path text NOT NULL,
    value_encrypted bytea NOT NULL,
    host_only boolean NOT NULL,
    secure boolean NOT NULL,
    http_only boolean NOT NULL,
    same_site integer NOT NULL DEFAULT 0,
    expires_at timestamptz,
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (context_id, name, domain, path)
);
CREATE INDEX proxy_routes_context_id_idx ON proxy_routes(context_id);
CREATE INDEX proxy_cookies_context_id_idx ON proxy_cookies(context_id);
CREATE INDEX proxy_sessions_expires_at_idx ON proxy_sessions(expires_at);

-- +goose Down
DROP TABLE proxy_cookies;
DROP TABLE proxy_sessions;
DROP TABLE proxy_tickets;
DROP TABLE proxy_routes;
DROP TABLE proxy_contexts;

