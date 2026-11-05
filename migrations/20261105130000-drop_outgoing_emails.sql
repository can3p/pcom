-- +migrate Up

-- Nothing writes outgoing_emails any more. A row an old machine queued while
-- 20261105121000-sent_emails.sql ran is handled the way that migration
-- handled the rest: unsent mail becomes a mail.rendered job, sent mail is
-- recorded in sent_emails so its dedup holds.
insert into river_job (args, kind, max_attempts, queue, state, scheduled_at)
select jsonb_build_object('unique_key', unique_id, 'email_type', email_type, 'mail', payload),
       'mail.rendered', 4, 'default', 'available', now()
from outgoing_emails
where status = 'new';

insert into sent_emails (id, unique_id, email_type, payload, sent_at, created_at)
select id, unique_id, email_type, payload, coalesce(sent_at, updated_at), created_at
from outgoing_emails
where status = 'sent'
on conflict (email_type, unique_id) do nothing;

drop table outgoing_emails;
drop type outgoing_email_status;

-- +migrate Down

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
  updated_at timestamp not null,
  email_type varchar not null
);

create unique index on outgoing_emails(email_type, unique_id);
