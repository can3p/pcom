
-- +migrate Up
create index on rss_feeds(disable_reason);

-- +migrate Down
