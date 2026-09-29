package mail_test

import (
	"context"
	"testing"
	"time"

	"github.com/can3p/pcom/pkg/mail"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/fakesender"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
)

// newUser and newInvitation wrap the factory calls this file repeats, so
// each test case reads as one line of setup.
func newUser(t *testing.T, ctx context.Context, db *sqlx.DB) *core.User {
	t.Helper()

	return testutil.Must(factory.User(ctx, db))(t)
}

func newInvitation(t *testing.T, ctx context.Context, db *sqlx.DB, userID string) {
	t.Helper()

	testutil.Must(factory.Invitation(ctx, db, userID))(t)
}

func TestSendInvite(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	t.Run("consumes the invite", func(t *testing.T) {
		t.Parallel()

		senderUser := newUser(t, ctx, db)
		newInvitation(t, ctx, db, senderUser.ID)
		s := fakesender.New()

		require.NoError(t, mail.SendInvite(ctx, db, s, senderUser, "first-invitee@example.test"))

		sent := s.Sent()
		require.Len(t, sent, 1)
		require.Equal(t, "user_invitation", sent[0].EmailType)
		require.Equal(t, "first-invitee@example.test", sent[0].Mail.To[0].Address)

		// The only invitation slot senderUser had is now used, so a second
		// invite fails: it proves SendInvite marked the row as used rather
		// than leaving it free to be picked again.
		err := mail.SendInvite(ctx, db, s, senderUser, "second-invitee@example.test")
		require.Error(t, err)
		require.Contains(t, err.Error(), "does not have any unused invites")
		require.Len(t, s.Sent(), 1, "no mail should have been sent for the rejected invite")
	})

	t.Run("fails without unused invite", func(t *testing.T) {
		t.Parallel()

		senderUser := newUser(t, ctx, db)
		s := fakesender.New()

		err := mail.SendInvite(ctx, db, s, senderUser, "invitee@example.test")
		require.Error(t, err)
		require.Contains(t, err.Error(), "does not have any unused invites")
		require.Empty(t, s.Sent())
	})

	t.Run("rejects existing email", func(t *testing.T) {
		t.Parallel()

		senderUser := newUser(t, ctx, db)
		newInvitation(t, ctx, db, senderUser.ID)
		existingUser := newUser(t, ctx, db)
		s := fakesender.New()

		err := mail.SendInvite(ctx, db, s, senderUser, existingUser.Email)
		require.Error(t, err)
		require.Contains(t, err.Error(), "already exists")
		require.Empty(t, s.Sent(), "no mail should have been sent for an existing email")

		// The invite slot is left untouched by the rejection: it can still be
		// used for a genuinely new email address.
		require.NoError(t, mail.SendInvite(ctx, db, s, senderUser, "new-invitee@example.test"))
		require.Len(t, s.Sent(), 1)
	})
}

func TestConfirmWaitingList_SetsVerificationSentAt(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	waitingList := testutil.Must(factory.SignupRequest(ctx, db))(t)
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

func TestValidate(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	t.Run("rejects existing user", func(t *testing.T) {
		t.Parallel()

		existingUser := newUser(t, ctx, db)

		err := mail.Validate(ctx, db, existingUser.Email)
		require.Error(t, err)
		require.Contains(t, err.Error(), "already registered")
	})

	t.Run("allows new email", func(t *testing.T) {
		t.Parallel()

		require.NoError(t, mail.Validate(ctx, db, "brand-new-user@example.test"))
	})
}
