-- +goose Up
CREATE TABLE direct_access_settings (
    singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton),
    restricted boolean NOT NULL DEFAULT false,
    updated_at timestamptz NOT NULL DEFAULT now()
);
INSERT INTO direct_access_settings(singleton, restricted) VALUES (true, false);
CREATE TABLE direct_access_users (
    user_id uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE
);
CREATE TABLE direct_access_ldap_groups (
    group_id uuid PRIMARY KEY REFERENCES ldap_groups(id) ON DELETE CASCADE
);

-- +goose Down
DROP TABLE direct_access_ldap_groups;
DROP TABLE direct_access_users;
DROP TABLE direct_access_settings;
