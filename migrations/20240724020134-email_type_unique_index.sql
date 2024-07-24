
-- +migrate Up
alter table outgoing_emails add column email_type varchar not null;

create unique index on outgoing_emails(email_type, unique_id);

-- +migrate Down
