
-- +migrate Up
alter table posts add column published_at timestamp;
update posts set published_at = created_at;
alter table posts alter column published_at set not null;

-- +migrate Down
