package mail_test

import (
	"context"
	"testing"
	"time"

	"github.com/can3p/pcom/pkg/mail"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/fakesender"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/stretchr/testify/require"
)

func TestSendInvite_ConsumesTheInvite(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	senderUser, err := factory.User(ctx, db)
	require.NoError(t, err)

	_, err = factory.Invitation(ctx, db, senderUser.ID)
	require.NoError(t, err)

	s := fakesender.New()

	require.NoError(t, mail.SendInvite(ctx, db, s, senderUser, "first-invitee@example.test"))

	sent := s.Sent()
	require.Len(t, sent, 1)
	require.Equal(t, "user_invitation", sent[0].EmailType)
	require.Equal(t, "first-invitee@example.test", sent[0].Mail.To[0].Address)

	// The only invitation slot senderUser had is now used, so a second
	// invite fails: it proves SendInvite marked the row as used rather
	// than leaving it free to be picked again.
	err = mail.SendInvite(ctx, db, s, senderUser, "second-invitee@example.test")
	require.Error(t, err)
	require.Contains(t, err.Error(), "does not have any unused invites")

	require.Len(t, s.Sent(), 1, "no mail should have been sent for the rejected invite")
}

func TestSendInvite_FailsWithoutUnusedInvite(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	senderUser, err := factory.User(ctx, db)
	require.NoError(t, err)

	s := fakesender.New()

	err = mail.SendInvite(ctx, db, s, senderUser, "invitee@example.test")
	require.Error(t, err)
	require.Contains(t, err.Error(), "does not have any unused invites")
	require.Empty(t, s.Sent())
}

func TestSendInvite_RejectsExistingEmail(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	senderUser, err := factory.User(ctx, db)
	require.NoError(t, err)

	_, err = factory.Invitation(ctx, db, senderUser.ID)
	require.NoError(t, err)

	existingUser, err := factory.User(ctx, db)
	require.NoError(t, err)

	s := fakesender.New()

	err = mail.SendInvite(ctx, db, s, senderUser, existingUser.Email)
	require.Error(t, err)
	require.Contains(t, err.Error(), "already exists")
	require.Empty(t, s.Sent(), "no mail should have been sent for an existing email")

	// The invite slot is left untouched by the rejection: it can still be
	// used for a genuinely new email address.
	require.NoError(t, mail.SendInvite(ctx, db, s, senderUser, "new-invitee@example.test"))
	require.Len(t, s.Sent(), 1)
}

func TestConfirmWaitingList_SetsVerificationSentAt(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	waitingList, err := factory.SignupRequest(ctx, db)
	require.NoError(t, err)
	require.False(t, waitingList.VerificationSentAt.Valid)

	s := fakesender.New()

	require.NoError(t, mail.ConfirmWaitingList(ctx, db, s, waitingList))

	require.True(t, waitingList.VerificationSentAt.Valid)
	require.WithinDuration(t, time.Now(), waitingList.VerificationSentAt.Time, 5*time.Second)

	sent := s.Sent()
	require.Len(t, sent, 1)
	require.Equal(t, "waiting_list_confirm", sent[0].EmailType)
	require.Equal(t, waitingList.Email, sent[0].Mail.To[0].Address)
}

func TestValidate_RejectsExistingUser(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	existingUser, err := factory.User(ctx, db)
	require.NoError(t, err)

	err = mail.Validate(ctx, db, existingUser.Email)
	require.Error(t, err)
	require.Contains(t, err.Error(), "already registered")
}

func TestValidate_AllowsNewEmail(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	require.NoError(t, mail.Validate(ctx, db, "brand-new-user@example.test"))
}
