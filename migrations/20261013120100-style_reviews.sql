-- +migrate Up

-- The style reviews of a post, numbered from 1 in the order they were made.
-- subject and body are the text that was reviewed, so the next review can
-- say what changed; author_note is what the author told the reviewer.
create table style_reviews (
  id uuid primary key,
  post_id uuid not null references posts(id) on delete cascade,
  number integer not null,
  subject text not null,
  body text not null,
  summary text not null,
  author_note text not null default '',
  created_at timestamp not null,
  unique (post_id, number)
);

-- +migrate Down

drop table style_reviews;
