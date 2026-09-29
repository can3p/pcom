-- +migrate Up

-- One pending invitation per address. Invitation emails are stored
-- normalized, so plain equality is enough.
CREATE UNIQUE INDEX user_invitations_pending_email_idx
ON user_invitations (invitation_email)
WHERE invitation_email IS NOT NULL AND created_user_id IS NULL;

-- +migrate Down

DROP INDEX user_invitations_pending_email_idx;
