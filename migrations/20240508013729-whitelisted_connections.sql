
-- +migrate Up
create table whitelisted_connections (
  id uuid primary key,
  who_id uuid references users(id) not null,
  allows_who_id uuid references users(id) not null,
  created_at timestamp not null,
  updated_at timestamp not null,
  connection_id uuid references user_connections(id)
);

create unique index on whitelisted_connections(who_id, allows_who_id) where connection_id is null;

-- +migrate Down

drop table whitelisted_connections;
