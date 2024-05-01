
-- +migrate Up
CREATE TABLE user_signup_requests (
  id uuid primary key,
  email varchar not null,
  reason varchar,
  signup_attribution varchar,
  created_user_id uuid references users(id),
  verification_sent_at timestamp,
  email_confirmed_at timestamp,
  created_at timestamp,
  updated_at timestamp
);

create unique index on user_signup_requests(email);

-- +migrate Down
