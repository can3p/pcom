
-- +migrate Up
-- RSS Feeds table
CREATE TABLE rss_feeds (
    id UUID NOT NULL PRIMARY KEY,
    url TEXT NOT NULL UNIQUE,
    title TEXT NOT NULL,
    description TEXT NOT NULL,
    last_fetched_at timestamp,
    avg_items_per_day FLOAT NOT NULL,
    last_items_count INTEGER NOT NULL DEFAULT 0,
    update_frequency_minutes INTEGER NOT NULL,
    next_fetch_at timestamp,
    last_manual_refresh_at timestamp,
    consecutive_empty_fetches INTEGER NOT NULL,
    created_at timestamp NOT NULL,
    updated_at timestamp NOT NULL
);

-- User subscriptions to RSS feeds
CREATE TABLE user_feed_subscriptions (
    id UUID NOT NULL PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id),
    feed_id UUID NOT NULL REFERENCES rss_feeds(id),
    created_at timestamp NOT NULL,
    updated_at timestamp NOT NULL,
    UNIQUE(user_id, feed_id)
);

-- URLs table for deduplication
CREATE TABLE normalized_urls (
    id UUID NOT NULL PRIMARY KEY,
    url TEXT NOT NULL,
    created_at timestamp NOT NULL,
    updated_at timestamp NOT NULL
);

-- RSS Items table
CREATE TABLE rss_items (
    id UUID NOT NULL PRIMARY KEY,
    feed_id UUID NOT NULL REFERENCES rss_feeds(id),
    url_id UUID NOT NULL REFERENCES normalized_urls(id),
    guid TEXT NOT NULL,
    title TEXT NOT NULL,
    description TEXT NOT NULL,
    sanitized_description TEXT NOT NULL,
    published_at timestamp NOT NULL,
    created_at timestamp NOT NULL,
    updated_at timestamp NOT NULL,
    UNIQUE(feed_id, url_id)
);

-- User's feed items
CREATE TABLE user_feed_items (
    id UUID NOT NULL PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id),
    rss_item_id UUID NOT NULL REFERENCES rss_items(id),
    url_id UUID NOT NULL REFERENCES normalized_urls(id),
    is_dismissed BOOLEAN NOT NULL,
    created_at timestamp NOT NULL,
    updated_at timestamp NOT NULL
);

-- Add columns to existing posts table
ALTER TABLE posts
    ADD COLUMN url_id UUID REFERENCES normalized_urls(id),
    ADD COLUMN rss_item_id UUID REFERENCES rss_items(id);

-- Create indexes
CREATE INDEX idx_user_feed_subscriptions_user_id ON user_feed_subscriptions(user_id);
CREATE INDEX idx_user_feed_items_user_id ON user_feed_items(user_id);
CREATE INDEX idx_user_feed_items_url_id ON user_feed_items(url_id);
CREATE INDEX idx_rss_items_feed_id ON rss_items(feed_id);
CREATE INDEX idx_rss_items_url_id ON rss_items(url_id);
CREATE INDEX idx_posts_url_id ON posts(url_id);
CREATE UNIQUE INDEX ON normalized_urls(url);

-- +migrate Down
