-- +goose Up
CREATE TABLE ldap_settings (
    singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton),
    enabled boolean NOT NULL DEFAULT false,
    mode text NOT NULL DEFAULT 'search_bind' CHECK (mode IN ('search_bind', 'dn_template')),
    url text NOT NULL DEFAULT '',
    start_tls boolean NOT NULL DEFAULT true,
    base_dn text NOT NULL DEFAULT '',
    bind_dn text NOT NULL DEFAULT '',
    user_filter text NOT NULL DEFAULT '(&(objectClass=person)(uid={{username}}))',
    user_dn_template text NOT NULL DEFAULT '',
    username_attribute text NOT NULL DEFAULT 'uid',
    display_name_attribute text NOT NULL DEFAULT 'cn',
    email_attribute text NOT NULL DEFAULT 'mail',
    group_mode text NOT NULL DEFAULT 'member_of' CHECK (group_mode IN ('member_of', 'search')),
    group_base_dn text NOT NULL DEFAULT '',
    group_filter text NOT NULL DEFAULT '(&(objectClass=groupOfNames)(member={{user_dn}}))',
    group_name_attribute text NOT NULL DEFAULT 'cn',
    updated_at timestamptz NOT NULL DEFAULT now()
);
INSERT INTO ldap_settings (singleton) VALUES (true);

CREATE TABLE ldap_groups (
    id uuid PRIMARY KEY,
    external_dn text NOT NULL UNIQUE,
    name text NOT NULL,
    last_seen_at timestamptz NOT NULL DEFAULT now(),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE user_ldap_groups (
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    group_id uuid NOT NULL REFERENCES ldap_groups(id) ON DELETE CASCADE,
    PRIMARY KEY (user_id, group_id)
);

-- +goose Down
DROP TABLE user_ldap_groups;
DROP TABLE ldap_groups;
DROP TABLE ldap_settings;

