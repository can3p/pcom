
-- +migrate Up
alter table user_styles alter column styles set not null;

-- +migrate Down
