
-- +migrate Up
alter table posts rename column visbility_radius to visibility_radius;

-- +migrate Down
