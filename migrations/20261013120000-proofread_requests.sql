-- +migrate Up

-- One row per call to Claude for a grammar check or a style review, ok or
-- failed. The hourly limits count a user's ok rows of a kind; the site-wide
-- daily cap counts every row. post_id is null for a check of a post that was
-- never saved, or once the post is deleted.
create table proofread_requests (
  id uuid primary key,
  user_id uuid not null references users(id) on delete cascade,
  kind text not null check (kind in ('grammar', 'style')),
  post_id uuid references posts(id) on delete set null,
  model text not null,
  input_tokens integer not null default 0,
  output_tokens integer not null default 0,
  status text not null check (status in ('ok', 'failed')),
  created_at timestamp not null
);

create index on proofread_requests(user_id, kind, created_at);

-- +migrate Down

drop table proofread_requests;
