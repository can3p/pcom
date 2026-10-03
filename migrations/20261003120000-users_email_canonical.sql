-- +migrate Up

-- The mailbox each address delivers to, so that signup can allow one account
-- per mailbox. The app computes it (pgsession.CanonicalEmail) on every
-- insert; the backfill below is a copy of that rule as it stood when this
-- migration was written: lowercased, no +tag, and for Gmail no dots and
-- googlemail.com as gmail.com.
alter table users add column email_canonical varchar;

update users
set email_canonical = case
  when split_part(lower(email), '@', 2) in ('gmail.com', 'googlemail.com')
    then replace(split_part(split_part(lower(email), '@', 1), '+', 1), '.', '') || '@gmail.com'
  else split_part(split_part(lower(email), '@', 1), '+', 1) || '@' || split_part(lower(email), '@', 2)
end;

alter table users alter column email_canonical set not null;

-- not unique: an invitation may go to a +tag of a registered address
create index users_email_canonical_idx on users (email_canonical);

-- +migrate Down

drop index users_email_canonical_idx;
alter table users drop column email_canonical;
