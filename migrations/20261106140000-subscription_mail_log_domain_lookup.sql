-- +migrate Up

-- A DNS lookup of a subscribing address's domain: the site-wide hourly lookup
-- budget counts them. Refused domains no longer get a row of their own.
delete from subscription_mail_log where kind = 'domain_refused';
alter table subscription_mail_log drop constraint subscription_mail_log_kind_check;
alter table subscription_mail_log add constraint subscription_mail_log_kind_check
  check (kind in ('confirm', 'manage_link', 'domain_lookup'));

-- +migrate Down

delete from subscription_mail_log where kind = 'domain_lookup';
alter table subscription_mail_log drop constraint subscription_mail_log_kind_check;
alter table subscription_mail_log add constraint subscription_mail_log_kind_check
  check (kind in ('confirm', 'manage_link', 'domain_refused'));
