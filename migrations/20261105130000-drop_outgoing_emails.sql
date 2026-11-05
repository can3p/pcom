-- +migrate Up

-- Nothing writes outgoing_emails any more, and no worker sends a mail
-- queued there: 20261105121000-sent_emails.sql turned the unsent rows into
-- jobs of a kind since retired. A row an old machine queued during that
-- deploy stops this migration rather than being dropped unsent.
-- +migrate StatementBegin
do $$
begin
  if exists (select 1 from outgoing_emails where status = 'new') then
    raise exception 'outgoing_emails still has unsent mail';
  end if;
end
$$;
-- +migrate StatementEnd

-- The sent mail sent_emails doesn't have yet, so its dedup stays complete.
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
