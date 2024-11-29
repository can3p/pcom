
-- +migrate Up
CREATE TABLE user_styles (
  id uuid primary key,
  user_id uuid not null references users(id),
  styles text CHECK (char_length(styles) <= 10000),
  created_at timestamp,
  updated_at timestamp
);

create unique index on user_styles(user_id);

-- +migrate Down
