-- +migrate Up

-- The text of a new post before it becomes a draft: one pad per user and
-- prompt (prompt_id null is the plain pad). revision is bumped by every write,
-- so two tabs on one pad are told apart by a conditional update. A pad that
-- became a post, or was discarded, is emptied rather than deleted and records
-- the post it became, so a stale tab can be told where its text went.
create table post_scratches (
  id uuid primary key,
  user_id uuid not null references users(id) on delete cascade,
  prompt_id uuid references post_prompts(id) on delete cascade,
  subject text,
  url text,
  body text not null default '',
  visibility post_visibility not null,
  notify_subscribers boolean,
  note_ids uuid[] not null default '{}',
  revision int not null default 0,
  promoted_post_id uuid references posts(id) on delete set null,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

-- coalesce rather than NULLS NOT DISTINCT, which needs Postgres 15; the
-- repository's upsert names the same expression as its conflict target.
create unique index post_scratches_user_prompt_idx
  on post_scratches (user_id, coalesce(prompt_id, '00000000-0000-0000-0000-000000000000'::uuid));

-- +migrate Down
drop table post_scratches;
