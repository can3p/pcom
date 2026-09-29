-- +migrate Up

-- A read-only token for the private RSS feed. It is separate from the API key
-- (which can write) because feed URLs get pasted into third-party readers and
-- end up in logs. One token per user; regenerating replaces it.
create table user_feed_tokens (
  id uuid primary key,
  user_id uuid references users(id) not null,
  token uuid not null,
  created_at timestamp not null default now(),
  updated_at timestamp not null default now()
);

create unique index on user_feed_tokens(user_id);
create unique index on user_feed_tokens(token);

-- +migrate Down

drop table user_feed_tokens;
