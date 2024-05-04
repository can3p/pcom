
-- +migrate Up
alter table posts alter column user_id set not null;

-- +migrate Down
