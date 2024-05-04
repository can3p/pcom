
-- +migrate Up
create table posts (
  id uuid primary key,
  subject varchar not null,
  body varchar not null,
  user_id uuid references users(id),
  created_at timestamp,
  updated_at timestamp
);

create index on posts(user_id);

-- +migrate Down
