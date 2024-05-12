
-- +migrate Up
CREATE TABLE post_comments (
  id uuid primary key,
  user_id uuid not null references users(id),
  post_id uuid not null references posts(id),
  parent_comment_id uuid references post_comments(id),
  body varchar not null,
  created_at timestamp not null,
  updated_at timestamp not null
);

create index on post_comments(post_id, parent_comment_id);

CREATE TABLE post_stats (
  id uuid primary key,
  post_id uuid not null references posts(id),
  comments_number bigint not null,
  created_at timestamp not null,
  updated_at timestamp not null
);

-- +migrate Down
