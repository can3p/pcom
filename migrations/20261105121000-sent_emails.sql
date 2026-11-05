-- +migrate Up

-- One row per mail pcom sent, with what it sent. The mail worker inserts the
-- row in the transaction that delivers the mail; (email_type, unique_id)
-- drops a repeat for good, as outgoing_emails' index did. unique_id is the
-- SHA-1 uuid of the mail's unique id string, as in outgoing_emails.
create table sent_emails (
  id uuid primary key,
  unique_id uuid not null,
  email_type varchar not null,
  payload jsonb not null,
  sent_at timestamp not null,
  created_at timestamp not null
);

create unique index on sent_emails(email_type, unique_id);

-- The mail outgoing_emails already sent.
insert into sent_emails (id, unique_id, email_type, payload, sent_at, created_at)
select id, unique_id, email_type, payload, coalesce(sent_at, updated_at), created_at
from outgoing_emails
where status = 'sent';

-- The mail it still had to send becomes a mail.rendered job: its args are the
-- rendered mail, sent as is. max_attempts is one more than the default
-- --email-retry-intervals has waits. The rows stay in outgoing_emails, marked
-- failed, so an old poller still running during the deploy doesn't send them
-- too.
insert into river_job (args, kind, max_attempts, queue, state, scheduled_at)
select jsonb_build_object('unique_key', unique_id, 'email_type', email_type, 'mail', payload),
       'mail.rendered', 4, 'default', 'available', now()
from outgoing_emails
where status = 'new';

update outgoing_emails set status = 'failed' where status = 'new';

-- +migrate Down

drop table sent_emails;
