-- +migrate Up

-- A subscribe attempt whose address's domain can't receive mail: it costs a
-- DNS lookup, so it counts toward the site-wide hourly brake.
alter table subscription_mail_log drop constraint subscription_mail_log_kind_check;
alter table subscription_mail_log add constraint subscription_mail_log_kind_check
  check (kind in ('confirm', 'manage_link', 'domain_refused'));

-- +migrate Down

delete from subscription_mail_log where kind = 'domain_refused';
alter table subscription_mail_log drop constraint subscription_mail_log_kind_check;
alter table subscription_mail_log add constraint subscription_mail_log_kind_check
  check (kind in ('confirm', 'manage_link'));
