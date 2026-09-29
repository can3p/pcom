-- +migrate Up

-- Emails are stored normalized (lowercase, no surrounding space) and
-- compared by plain equality. users.email and user_signup_requests.email
-- already have unique indexes, so normalizing two rows into the same address
-- makes this migration fail rather than merge or skip them: such accounts
-- have to be resolved by hand first.
UPDATE users
SET email = lower(btrim(email))
WHERE email <> lower(btrim(email));

UPDATE user_invitations
SET invitation_email = lower(btrim(invitation_email))
WHERE invitation_email <> lower(btrim(invitation_email));

UPDATE user_signup_requests
SET email = lower(btrim(email))
WHERE email <> lower(btrim(email));

-- The format check is deliberately loose, one "@" with text on both sides:
-- the application validates addresses properly before storing them.
ALTER TABLE users
  ADD CONSTRAINT users_email_normalized CHECK (email = lower(btrim(email))),
  ADD CONSTRAINT users_email_format CHECK (email ~ '^[^@]+@[^@]+$');

ALTER TABLE user_invitations
  ADD CONSTRAINT user_invitations_invitation_email_normalized
    CHECK (invitation_email = lower(btrim(invitation_email))),
  ADD CONSTRAINT user_invitations_invitation_email_format
    CHECK (invitation_email ~ '^[^@]+@[^@]+$');

ALTER TABLE user_signup_requests
  ADD CONSTRAINT user_signup_requests_email_normalized CHECK (email = lower(btrim(email))),
  ADD CONSTRAINT user_signup_requests_email_format CHECK (email ~ '^[^@]+@[^@]+$');

-- +migrate Down

ALTER TABLE user_signup_requests
  DROP CONSTRAINT user_signup_requests_email_format,
  DROP CONSTRAINT user_signup_requests_email_normalized;

ALTER TABLE user_invitations
  DROP CONSTRAINT user_invitations_invitation_email_format,
  DROP CONSTRAINT user_invitations_invitation_email_normalized;

ALTER TABLE users
  DROP CONSTRAINT users_email_format,
  DROP CONSTRAINT users_email_normalized;
