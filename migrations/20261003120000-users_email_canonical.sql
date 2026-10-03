-- +migrate Up

-- The mailbox each address delivers to, so that signup can allow one account
-- per mailbox. The app computes it (pgsession.CanonicalEmail) on every
-- insert; the backfill below is a copy of that rule as it stood when this
-- migration was written: trimmed and lowercased, no +tag, and for Gmail no dots and
-- googlemail.com as gmail.com.
alter table users add column email_canonical varchar;

update users
set email_canonical = case
  when domain is null then e
  when domain in ('gmail.com', 'googlemail.com') then replace(local, '.', '') || '@gmail.com'
  else local || '@' || domain
end
from (
  -- the last @ splits the address, as in Go; .* is greedy
  select id, e,
         split_part(substring(e from '^(.*)@'), '+', 1) as local,
         substring(e from '@([^@]*)$') as domain
  from (select id, lower(trim(email)) as e from users) as lowered
) as parts
where parts.id = users.id;

alter table users alter column email_canonical set not null;

-- not unique: an invitation may go to a +tag of a registered address
create index users_email_canonical_idx on users (email_canonical);

-- +migrate Down

drop index users_email_canonical_idx;
alter table users drop column email_canonical;
