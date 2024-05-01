
-- +migrate Up
CREATE TABLE user_invitations (
  id uuid primary key,
  user_id uuid not null references users(id),
  invitation_email varchar,
  invitation_sent_at timestamp,
  created_at timestamp,
  updated_at timestamp,
  created_user_id UUID REFERENCES users(id)
);

-- +migrate Down
