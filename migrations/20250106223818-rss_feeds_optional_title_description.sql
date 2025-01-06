
-- +migrate Up
alter table rss_feeds
alter column title drop not null,
alter column description drop not null;

-- +migrate Down
