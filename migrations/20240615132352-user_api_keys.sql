
-- +migrate Up
create table user_api_keys (
  id uuid primary key,
  api_key uuid not null,
  user_id uuid references users(id) not null,
  created_at timestamp not null,
  updated_at timestamp not null
);

-- not doing anything fancy with key rotation at the moment
create unique index on user_api_keys(user_id);

-- +migrate Down
