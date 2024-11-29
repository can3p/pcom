
-- +migrate Up
create type profile_visibility as ENUM ('connections', 'registered_users', 'public');
alter table users add column profile_visibility profile_visibility not null default 'registered_users';

-- +migrate Down
alter table users drop column profile_visibility;
drop type profile_visibility;
