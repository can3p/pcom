
-- +migrate Up
CREATE TABLE system_settings (
  id uuid not null primary key,
  registration_open boolean not null
);

INSERT INTO system_settings (id, registration_open) values ('c85780e1-dc5e-4033-a049-c68906b24d1d', true);

-- +migrate Down
DROP TABLE system_settings;
