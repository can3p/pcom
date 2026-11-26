-- +migrate Up

-- revision is bumped by every write of a post's text, so two tabs editing one
-- post are told apart by a conditional update, as on post_scratches.
alter table posts add column revision int not null default 1;

-- +migrate Down
alter table posts drop column revision;
