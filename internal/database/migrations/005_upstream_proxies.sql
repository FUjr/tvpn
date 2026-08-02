-- +goose Up
CREATE TABLE upstream_proxies (
    id uuid PRIMARY KEY,
    name text NOT NULL UNIQUE,
    proxy_type text NOT NULL CHECK (proxy_type IN ('http','socks5')),
    host text NOT NULL,
    port integer NOT NULL CHECK (port BETWEEN 1 AND 65535),
    username text NOT NULL DEFAULT '',
    password_encrypted bytea,
    enabled boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE user_upstream_proxies (
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    upstream_proxy_id uuid NOT NULL REFERENCES upstream_proxies(id) ON DELETE CASCADE,
    PRIMARY KEY (user_id, upstream_proxy_id)
);
CREATE TABLE ldap_group_upstream_proxies (
    group_id uuid NOT NULL REFERENCES ldap_groups(id) ON DELETE CASCADE,
    upstream_proxy_id uuid NOT NULL REFERENCES upstream_proxies(id) ON DELETE CASCADE,
    PRIMARY KEY (group_id, upstream_proxy_id)
);
ALTER TABLE proxy_contexts ADD COLUMN upstream_proxy_id uuid REFERENCES upstream_proxies(id);
CREATE INDEX proxy_contexts_upstream_proxy_id_idx ON proxy_contexts(upstream_proxy_id);

-- +goose Down
DROP INDEX proxy_contexts_upstream_proxy_id_idx;
ALTER TABLE proxy_contexts DROP COLUMN upstream_proxy_id;
DROP TABLE ldap_group_upstream_proxies;
DROP TABLE user_upstream_proxies;
DROP TABLE upstream_proxies;
