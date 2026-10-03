-- +migrate Up

-- canonical_email is the mailbox an address delivers to, so that signup can
-- allow one account per mailbox: the address lowercased, without a +tag, and
-- for Gmail without the dots it ignores and with googlemail.com as gmail.com.
-- It is the only definition; the service compares through it, never in Go.
-- +migrate StatementBegin
create function canonical_email(email text) returns text
language sql immutable strict parallel safe as $$
  select case
    when domain in ('gmail.com', 'googlemail.com') then replace(local, '.', '') || '@gmail.com'
    else local || '@' || domain
  end
  from (
    select split_part(substring(e from '^(.*)@'), '+', 1) as local,
           substring(e from '@([^@]*)$') as domain
    from (select lower(trim(email)) as e) as lowered
  ) as parts
$$;
-- +migrate StatementEnd

-- not unique: an invitation may go to a +tag of a registered address
create index users_canonical_email_idx on users (canonical_email(email));

-- +migrate Down

drop index users_canonical_email_idx;
drop function canonical_email(text);
