
-- +migrate Up
alter table user_connection_mediation_requests add column source_note varchar;

-- +migrate Down
