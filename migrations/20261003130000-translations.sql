
-- +migrate Up

-- The language a post or RSS item is written in, as ISO 639-1; null when
-- detection was unsure. A post is translated for readers only when its
-- author allows it.
alter table posts
  add column language varchar(2),
  add column allow_translation boolean not null default false;

alter table rss_items add column language varchar(2);

create type translation_source_kind as enum ('post', 'rss_item');

-- One cached translation per source and target language. The source is a
-- post or an RSS item, each through its own foreign key, so deleting either
-- deletes its translations. provider is the backend's Name, so switching
-- backends keeps the cache; source_hash (hex SHA-256 of the source subject
-- and body) tells a stale translation.
create table translations (
  id uuid primary key,
  source_kind translation_source_kind not null,
  post_id uuid references posts(id) on delete cascade,
  rss_item_id uuid references rss_items(id) on delete cascade,
  target_lang varchar(2) not null,
  source_hash text not null,
  provider text not null,
  subject text not null,
  body text not null,
  chars int not null,
  created_at timestamp not null default now(),
  updated_at timestamp not null default now(),
  constraint translations_source_check check (
    (source_kind = 'post' and post_id is not null and rss_item_id is null) or
    (source_kind = 'rss_item' and rss_item_id is not null and post_id is null)
  )
);

-- Nulls are distinct, so each index constrains only its own kind.
create unique index translations_post_target on translations(post_id, target_lang);
create unique index translations_rss_item_target on translations(rss_item_id, target_lang);

-- Every call to a backend, for the budgets: user_id is the reader who asked
-- (counted against their daily and the site's monthly limit), null for a
-- background re-translation (the site's monthly limit only). Cache hits make
-- no row. Kept apart from translations so that deleting a post or replacing a
-- translation doesn't give the budget back.
create table translation_usage (
  id uuid primary key,
  user_id uuid references users(id) on delete set null,
  provider text not null,
  chars int not null,
  created_at timestamp not null default now()
);

create index translation_usage_user_created on translation_usage(user_id, created_at);
create index translation_usage_created on translation_usage(created_at);

create type translation_job_status as enum ('new', 'failed');

-- The queue of re-translations of an edited post, sent by the worker the way
-- outgoing_emails are: a row waits until try_at, is deleted when done and
-- marked failed after its last attempt. One pending job per post and target.
create table translation_jobs (
  id uuid primary key,
  post_id uuid not null references posts(id) on delete cascade,
  target_lang varchar(2) not null,
  status translation_job_status not null default 'new',
  attempts_number int not null default 0,
  try_at timestamp not null default now(),
  created_at timestamp not null default now(),
  updated_at timestamp not null default now()
);

create unique index translation_jobs_post_target on translation_jobs(post_id, target_lang);
create index translation_jobs_status_try_at on translation_jobs(status, try_at);

-- The source languages a user always wants translated (to English, for now).
create table user_translation_languages (
  user_id uuid not null references users(id) on delete cascade,
  source_lang varchar(2) not null,
  created_at timestamp not null default now(),
  primary key (user_id, source_lang)
);

-- +migrate Down

drop table user_translation_languages;
drop table translation_jobs;
drop type translation_job_status;
drop table translation_usage;
drop table translations;
drop type translation_source_kind;
alter table rss_items drop column language;
alter table posts drop column allow_translation, drop column language;
