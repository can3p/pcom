
-- +migrate Up
alter table post_comments add top_comment_id uuid references post_comments(id);
update post_comments set top_comment_id = id where parent_comment_id is null;

-- +migrate Down
