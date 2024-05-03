
-- +migrate Up
alter table users add column username varchar not null;
create unique index on users(username);

-- +migrate Down
