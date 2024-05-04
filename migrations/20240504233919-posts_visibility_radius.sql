
-- +migrate Up
create type post_visibility as ENUM ('direct_only', 'second_degree');
alter table posts add column visbility_radius post_visibility;
update posts set visbility_radius = 'direct_only';
alter table posts alter column visbility_radius set not null;

-- +migrate Down
