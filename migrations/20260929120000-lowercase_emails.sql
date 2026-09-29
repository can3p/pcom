-- +migrate Up

-- Emails are compared ignoring case and surrounding space. users.email keeps
-- its stored spelling: legacy password hashes were computed from it, and some
-- legacy accounts differ only in case, so the index can't be unique. Login
-- normalizes an account's email once its hash is upgraded.
CREATE INDEX users_email_normalized_idx ON users (lower(btrim(email)));

-- An invitation's email is only ever read to create the account, which
-- normalizes it anyway; storing it normalized lets invitations be checked
-- for duplicates by plain equality.
UPDATE user_invitations
SET invitation_email = lower(btrim(invitation_email))
WHERE invitation_email <> lower(btrim(invitation_email));

-- +migrate Down

DROP INDEX users_email_normalized_idx;
