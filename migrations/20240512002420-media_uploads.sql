
-- +migrate Up
CREATE TABLE media_uploads (
  id uuid primary key,
  user_id uuid not null references users(id),
  uploaded_fname varchar not null,
  content_type varchar not null,
  created_at timestamp not null default now(),
  updated_at timestamp not null default now()
);

-- +migrate Down
