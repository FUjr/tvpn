-- +goose Up
CREATE TABLE policies (
    id uuid PRIMARY KEY,
    name text NOT NULL UNIQUE,
    description text NOT NULL DEFAULT '',
    mode text NOT NULL CHECK (mode IN ('deny_intranet', 'whitelist', 'blacklist', 'deny_all')),
    enabled boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE policy_rules (
    id uuid PRIMARY KEY,
    policy_id uuid NOT NULL REFERENCES policies(id) ON DELETE CASCADE,
    kind text NOT NULL CHECK (kind IN ('exact_host', 'domain_suffix', 'cidr', 'url_prefix')),
    value text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (policy_id, kind, value)
);
CREATE TABLE user_policies (
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    policy_id uuid NOT NULL REFERENCES policies(id) ON DELETE CASCADE,
    PRIMARY KEY (user_id, policy_id)
);
CREATE TABLE ldap_group_policies (
    group_id uuid NOT NULL REFERENCES ldap_groups(id) ON DELETE CASCADE,
    policy_id uuid NOT NULL REFERENCES policies(id) ON DELETE CASCADE,
    PRIMARY KEY (group_id, policy_id)
);
CREATE TABLE audit_events (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    actor_user_id uuid REFERENCES users(id) ON DELETE SET NULL,
    event_type text NOT NULL,
    outcome text NOT NULL CHECK (outcome IN ('allowed', 'denied', 'success', 'failure')),
    target text NOT NULL DEFAULT '',
    detail jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX audit_events_created_at_idx ON audit_events(created_at DESC);

-- +goose Down
DROP TABLE audit_events;
DROP TABLE ldap_group_policies;
DROP TABLE user_policies;
DROP TABLE policy_rules;
DROP TABLE policies;

