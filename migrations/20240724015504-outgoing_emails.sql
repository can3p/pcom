
-- +migrate Up
create type outgoing_email_status as ENUM ('new', 'sent', 'failed');

create table outgoing_emails (
  id uuid primary key,
  unique_id uuid not null,
  payload jsonb not null,
  status outgoing_email_status not null,
  attempts_number int not null,
  try_at timestamp not null,
  sent_at timestamp,
  created_at timestamp not null,
  updated_at timestamp not null
);

-- +migrate Down
