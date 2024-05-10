
-- +migrate Up
create unique index on whitelisted_connections(connection_id);

-- +migrate Down
