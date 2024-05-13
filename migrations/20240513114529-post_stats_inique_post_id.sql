
-- +migrate Up
create unique index on post_stats(post_id);

-- +migrate Down
