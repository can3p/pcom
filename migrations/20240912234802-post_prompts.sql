
-- +migrate Up
create table post_prompts (
  id uuid primary key,
  asker_id uuid not null references users(id),
  recipient_id uuid not null references users(id),
  message text not null,
  dismissed_at timestamp,
  post_id uuid references users(id),
  created_at timestamp not null,
  updated_at timestamp not null
);

-- +migrate Down
