-- +migrate Up
ALTER TABLE users DROP COLUMN pwdhash;

-- +migrate Down
ALTER TABLE users ADD COLUMN pwdhash text;
