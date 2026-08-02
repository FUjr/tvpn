-- +goose Up
ALTER TABLE proxy_contexts ADD COLUMN compatibility_mode boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE proxy_contexts DROP COLUMN compatibility_mode;
