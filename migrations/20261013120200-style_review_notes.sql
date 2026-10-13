-- +migrate Up

-- The notes of a style review, in the model's order (position). A note is
-- open until the author marks it done or kept (keep_reason says why), or the
-- next review settles it: carried when a note of the new review continues it
-- (continues_id), done otherwise.
create table style_review_notes (
  id uuid primary key,
  review_id uuid not null references style_reviews(id) on delete cascade,
  position integer not null,
  field text not null,
  kind text not null,
  quote text not null,
  before text not null,
  text text not null,
  continues_id uuid references style_review_notes(id) on delete set null,
  status text not null check (status in ('open', 'done', 'kept', 'carried')),
  keep_reason text not null default '',
  updated_at timestamp not null
);

create index on style_review_notes(review_id);

-- +migrate Down

drop table style_review_notes;
