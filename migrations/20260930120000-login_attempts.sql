-- +migrate Up

-- One row per login started with an email. The id goes into the session, and
-- the code mailed for it is stored only as an HMAC. user_id is null when the
-- address belongs to no confirmed user, so such an attempt can never log in;
-- code_hash is null when no code was issued (over the mail limit).
create table login_attempts (
  id uuid primary key,
  user_id uuid references users(id) on delete cascade,
  code_hash text,
  return_url text not null default '',
  tries integer not null default 0,
  expires_at timestamp not null,
  used_at timestamp,
  created_at timestamp not null default now(),
  updated_at timestamp not null default now()
);

create index on login_attempts(user_id, created_at);

-- +migrate Down

drop table login_attempts;
