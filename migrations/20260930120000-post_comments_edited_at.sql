-- +migrate Up

alter table post_comments add edited_at timestamp;

-- +migrate Down

alter table post_comments drop column edited_at;
