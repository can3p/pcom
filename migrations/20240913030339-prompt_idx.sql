
-- +migrate Up
create unique index on post_prompts(post_id);

-- +migrate Down
