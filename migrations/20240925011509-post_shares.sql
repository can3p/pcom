
-- +migrate Up
create table post_shares (
  id uuid primary key,
  post_id uuid not null references posts(id),
  created_at timestamp not null,
  updated_at timestamp not null
);

create unique index on post_shares(post_id);

-- +migrate Down
