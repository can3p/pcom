
-- +migrate Up
alter table post_comments alter column top_comment_id set not null;

-- +migrate Down
