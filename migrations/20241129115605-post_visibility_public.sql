
-- +migrate Up
alter type post_visibility add value 'public';

-- +migrate Down
