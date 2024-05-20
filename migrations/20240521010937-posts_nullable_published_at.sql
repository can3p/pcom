
-- +migrate Up
alter table posts alter column published_at drop not null;

-- +migrate Down
