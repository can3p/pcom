package mail_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/can3p/pcom/pkg/mail"
	"github.com/can3p/pcom/pkg/mail/sender/dbsender"
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

// TestSendInvite_Queue goes through the real email queue, which the fake
// sender doesn't imitate: the queue drops a mail whose (type, unique key)
// it has already seen.
func TestSendInvite_Queue(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	queued := func(t *testing.T, db *sqlx.DB) []string {
		t.Helper()
		var to []string
		for _, e := range testutil.Must(factory.ListOutgoingEmails(ctx, db, core.OutgoingEmailWhere.EmailType.EQ("user_invitation")))(t) {
			to = append(to, string(e.Payload))
		}

		return to
	}

	t.Run("a plus address is invited although the plain one is registered", func(t *testing.T) {
		t.Parallel()

		db := testdb.New(t).DB
		inviter := newUser(t, ctx, db)
		newInvitation(t, ctx, db, inviter.ID)
		testutil.Must(factory.User(ctx, db, factory.WithEmail("john.doe@mail.test")))(t)

		require.NoError(t, mail.SendInvite(ctx, db, dbsender.NewSender(db, fakesender.New()), inviter, "john.doe+prefix@mail.test"))
		to := queued(t, db)
		require.Len(t, to, 1)
		require.Contains(t, to[0], "john.doe+prefix@mail.test")
	})

	t.Run("a second pending invitation to the same address is rejected", func(t *testing.T) {
		t.Parallel()

		db := testdb.New(t).DB
		inviter := newUser(t, ctx, db)
		newInvitation(t, ctx, db, inviter.ID)
		newInvitation(t, ctx, db, inviter.ID)
		queue := dbsender.NewSender(db, fakesender.New())

		require.NoError(t, mail.SendInvite(ctx, db, queue, inviter, "same@example.test"))
		require.Error(t, mail.SendInvite(ctx, db, queue, inviter, " SAME@example.test "))
		require.Len(t, queued(t, db), 1)
	})

	t.Run("every invitation from one user queues its own mail", func(t *testing.T) {
		t.Parallel()

		db := testdb.New(t).DB
		inviter := newUser(t, ctx, db)
		newInvitation(t, ctx, db, inviter.ID)
		newInvitation(t, ctx, db, inviter.ID)
		queue := dbsender.NewSender(db, fakesender.New())

		require.NoError(t, mail.SendInvite(ctx, db, queue, inviter, "first@example.test"))
		require.NoError(t, mail.SendInvite(ctx, db, queue, inviter, "second@example.test"))
		to := queued(t, db)
		require.Len(t, to, 2)
		require.Contains(t, to[0], "first@example.test")
		require.Contains(t, to[1], "second@example.test")
	})

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

		for _, email := range []string{existingUser.Email, " " + strings.ToUpper(existingUser.Email) + " "} {
			err := mail.Validate(ctx, db, email)
			require.Error(t, err, email)
			require.Contains(t, err.Error(), "already registered")
		}
	})

	t.Run("allows new email", func(t *testing.T) {
		t.Parallel()

		require.NoError(t, mail.Validate(ctx, db, "brand-new-user@example.test"))
	})
}

func TestPendingInvitationUniqueIndex(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := testdb.New(t).DB
	inviter := newUser(t, ctx, db)

	t.Run("a second pending invitation to the same address fails", func(t *testing.T) {
		t.Parallel()

		testutil.Must(factory.Invitation(ctx, db, inviter.ID, factory.Sent("index-dup@example.test")))(t)
		_, err := factory.Invitation(ctx, db, inviter.ID, factory.Sent("index-dup@example.test"))
		require.Error(t, err)
	})

	t.Run("the address is free again once the first invitation is used", func(t *testing.T) {
		t.Parallel()

		used := testutil.Must(factory.User(ctx, db))(t)
		testutil.Must(factory.Invitation(ctx, db, inviter.ID, factory.Sent("index-used@example.test"), factory.UsedBy(used.ID)))(t)
		_, err := factory.Invitation(ctx, db, inviter.ID, factory.Sent("index-used@example.test"))
		require.NoError(t, err)
	})
}

// TestEmailConstraints: every stored email is normalized (lowercase, no
// surrounding space) and has one "@" with text on both sides, so lookups can
// compare by plain equality; users.email is unique.
func TestEmailConstraints(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := testdb.New(t).DB
	inviter := newUser(t, ctx, db)

	insert := map[string]func(email string) error{
		"user": func(email string) error {
			_, err := factory.User(ctx, db, factory.WithEmail(email))
			return err
		},
		"invitation": func(email string) error {
			_, err := factory.Invitation(ctx, db, inviter.ID, factory.Sent(email))
			return err
		},
		"signup request": func(email string) error {
			_, err := factory.SignupRequest(ctx, db, func(r *core.UserSignupRequest) { r.Email = email })
			return err
		},
	}

	bad := []string{"Mixed@example.test", " space@example.test", "space@example.test ", "no-at.example.test", "two@at@example.test", "@example.test", "nobody@"}

	for table, insert := range insert {
		for i, email := range bad {
			require.Error(t, insert(email), "%s %q", table, email)
			require.NoError(t, insert(fmt.Sprintf("ok-%d@%s.test", i, strings.ReplaceAll(table, " ", "-"))), table)
		}
	}

	testutil.Must(factory.User(ctx, db, factory.WithEmail("taken@example.test")))(t)
	_, err := factory.User(ctx, db, factory.WithEmail("taken@example.test"))
	require.Error(t, err, "users.email is unique")
}
