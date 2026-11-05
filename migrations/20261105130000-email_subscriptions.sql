-- +migrate Up

-- A reader who gets an author's new posts by email, without an account.
-- email_canonical is the mailbox the address delivers to (emailaddr.Parse),
-- computed by the service on insert. manage_token opens the page where the
-- reader sees and ends their subscriptions.
create table email_subscribers (
  id uuid primary key,
  email text not null unique,
  email_canonical text not null,
  manage_token text not null unique,
  created_at timestamp not null default now()
);

create index on email_subscribers(email_canonical);

-- One subscriber's subscription to one author. It is pending until the
-- subscriber follows the link mailed with confirm_token, then active.
create table email_subscriptions (
  id uuid primary key,
  subscriber_id uuid not null references email_subscribers(id) on delete cascade,
  author_id uuid not null references users(id) on delete cascade,
  status text not null check (status in ('pending', 'active')),
  confirm_token text not null unique,
  confirm_sent_at timestamp not null,
  confirmed_at timestamp,
  created_at timestamp not null default now(),
  unique (subscriber_id, author_id)
);

create index on email_subscriptions(author_id, status);

-- One row per confirmation or manage-link mail sent, so the service can count
-- them per address and site-wide; old rows are pruned by sent_at.
create table subscription_mail_log (
  email_canonical text not null,
  kind text not null check (kind in ('confirm', 'manage_link')),
  sent_at timestamp not null
);

create index on subscription_mail_log(email_canonical, kind, sent_at);
create index on subscription_mail_log(kind, sent_at);
create index on subscription_mail_log(sent_at);

-- The author lets readers subscribe by email; off until they turn it on.
alter table users add column email_subscriptions_enabled boolean not null default false;

-- When the post's subscribers were mailed, so a post is mailed once.
alter table posts add column subscribers_mailed_at timestamp;

-- +migrate Down

alter table posts drop column subscribers_mailed_at;
alter table users drop column email_subscriptions_enabled;
drop table subscription_mail_log;
drop table email_subscriptions;
drop table email_subscribers;
