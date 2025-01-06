
-- +migrate Up
create type rss_feed_disable_reason as ENUM ('fetch_failure', 'no_subscribers');

alter table rss_feeds
add column last_fetch_error text,
add column disable_reason rss_feed_disable_reason;

-- +migrate Down
