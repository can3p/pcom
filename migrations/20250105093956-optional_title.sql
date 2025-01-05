
-- +migrate Up
alter table posts alter column subject drop not null;

-- +migrate Down
