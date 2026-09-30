package accounts_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/can3p/gogo/sender"
	"github.com/can3p/pcom/pkg/mail/sender/dbsender"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service"
	"github.com/can3p/pcom/pkg/service/accounts"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/fakesender"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
)

// svcWith is the accounts service over db that sends through snd.
func svcWith(db *sqlx.DB, snd sender.Sender) *accounts.Service {
	return accounts.New(repo.New(db), snd, nil)
}

func acceptInvite(ctx context.Context, db *sqlx.DB, s sender.Sender, invite *core.UserInvitation, username, password string) error {
	_, err := svcWith(db, s).AcceptInvite(ctx, invite, username, password)

	return err
}

func sendInvite(ctx context.Context, db *sqlx.DB, s sender.Sender, inviter *core.User, to string) error {
	return svcWith(db, s).SendInvite(ctx, inviter, to)
}

// problem splits a check's result into the reason shown to the user and
// whether the input passed. Any other error fails the test run loudly.
func problem(err error) (string, bool) {
	var invalid *service.ValidationError
	if errors.As(err, &invalid) {
		return invalid.Message, false
	}

	if err != nil {
		panic(err)
	}

	return "", true
}

func newUser(t *testing.T, ctx context.Context, db *sqlx.DB) *core.User {
	t.Helper()

	return testutil.Must(factory.User(ctx, db))(t)
}

func newInvitation(t *testing.T, ctx context.Context, db *sqlx.DB, userID string) {
	t.Helper()

	testutil.Must(factory.Invitation(ctx, db, userID))(t)
}

func TestSignup_InsertsUnconfirmedUserAndNotifiesAdmin(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	sender := fakesender.New()

	u, err := svcWith(db, sender).Signup(ctx, "new-signup@example.test", "newsignup", "s3cr3t-pw", "some-campaign")
	require.NoError(t, err)
	require.NotEmpty(t, u.ID)

	got := testutil.Must(factory.GetUser(ctx, db, u.ID))(t)
	require.Equal(t, "new-signup@example.test", got.Email)
	require.Equal(t, "newsignup", got.Username)
	require.False(t, got.EmailConfirmedAt.Valid, "signup leaves the email unconfirmed until it's verified")
	require.True(t, got.EmailConfirmSeed.Valid, "a confirmation seed is generated so the user can confirm later")
	require.Equal(t, "some-campaign", got.SignupAttribution.String)

	require.Len(t, sender.Sent(), 1, "signup notifies the admin of the new user")
}

func TestSignup_MissingFieldsReturnsError(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	sender := fakesender.New()

	_, err := svcWith(db, sender).Signup(ctx, "", "user", "pw", "")
	require.Error(t, err)

	_, err = svcWith(db, sender).Signup(ctx, "a@b.example.test", "", "pw", "")
	require.Error(t, err)

	_, err = svcWith(db, sender).Signup(ctx, "a@b.example.test", "user", "", "")
	require.Error(t, err)

	require.Empty(t, sender.Sent(), "a rejected signup must not notify anyone")
}

func TestSignup_DuplicateEmailReturnsError(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	sender := fakesender.New()

	existing := testutil.Must(factory.User(ctx, db))(t)

	_, err := svcWith(db, sender).Signup(ctx, existing.Email, "someoneelse", "s3cr3t-pw", "")
	require.Error(t, err, "the email column is unique, so a second signup for it must fail")
	require.Empty(t, sender.Sent(), "a failed signup must not notify anyone")
}

func TestAcceptInvite_CreatesUserAndConnectsToInviter(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	sender := fakesender.New()

	inviter := testutil.Must(factory.User(ctx, db))(t)
	invite := testutil.Must(factory.Invitation(ctx, db, inviter.ID, factory.Sent("invitee@example.test")))(t)

	require.NoError(t, acceptInvite(ctx, db, sender, invite, "invitee", "s3cr3t-pw"))

	// AcceptInvite mutates the invite it was given: CreatedUserID is set to
	// the freshly inserted user's ID.
	require.True(t, invite.CreatedUserID.Valid)
	newUserID := invite.CreatedUserID.String

	gotUser := testutil.Must(factory.GetUser(ctx, db, newUserID))(t)
	require.Equal(t, "invitee", gotUser.Username)
	require.Equal(t, "invitee@example.test", gotUser.Email)
	require.True(t, gotUser.EmailConfirmedAt.Valid, "accepting an invite confirms the email right away")
	require.Equal(t, "accepted_invite", gotUser.SignupAttribution.String)

	require.True(t, testutil.Must(factory.ConnectionExists(ctx, db, inviter.ID, newUserID))(t), "accepting an invite connects the new user to the inviter")

	require.Len(t, sender.Sent(), 1, "accepting an invite notifies the admin of the new user")
}

func TestAcceptInvite_MissingFieldsReturnsError(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	sender := fakesender.New()

	inviter := testutil.Must(factory.User(ctx, db))(t)
	invite := testutil.Must(factory.Invitation(ctx, db, inviter.ID, factory.Sent("invitee2@example.test")))(t)

	require.Error(t, acceptInvite(ctx, db, sender, invite, "", "s3cr3t-pw"))
	require.Error(t, acceptInvite(ctx, db, sender, invite, "invitee2", ""))

	require.False(t, invite.CreatedUserID.Valid, "a rejected accept must not consume the invite")
	require.Empty(t, sender.Sent())
}

func TestAcceptInvite_DuplicateEmailReturnsError(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	sender := fakesender.New()

	inviter := testutil.Must(factory.User(ctx, db))(t)
	existing := testutil.Must(factory.User(ctx, db))(t)
	invite := testutil.Must(factory.Invitation(ctx, db, inviter.ID, factory.Sent(existing.Email)))(t)

	err := acceptInvite(ctx, db, sender, invite, "someoneelse", "s3cr3t-pw")
	require.Error(t, err, "the email column is unique, so accepting into an already-used email must fail")
	require.False(t, invite.CreatedUserID.Valid)
	require.Empty(t, sender.Sent())
}

func TestCheckCredentials(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	user := testutil.Must(factory.User(ctx, db, factory.WithPassword("correct-horse")))(t)

	require.NoError(t, svcWith(db, nil).CheckCredentials(ctx, user.Email, "correct-horse"))
	require.Error(t, svcWith(db, nil).CheckCredentials(ctx, user.Email, "wrong-password"))
	require.Error(t, svcWith(db, nil).CheckCredentials(ctx, "unknown@example.test", "correct-horse"))
}

func TestCheckCredentials_DBErrorIsReturnedAsIs(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	testDB := testdb.New(t)
	db := testDB.DB
	require.NoError(t, db.Close())

	err := svcWith(db, nil).CheckCredentials(ctx, "someone@example.test", "pw")
	require.Error(t, err)
	require.NotEqual(t, "Bad credentials", err.Error(), "a real DB error must not be mistaken for wrong credentials")
}

func TestSignupAndAcceptInvite_ReturnAdminNotificationError(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	boom := errors.New("smtp down")

	cases := []struct {
		name string
		run  func(s *fakesender.Sender) error
	}{
		{name: "signup", run: func(s *fakesender.Sender) error {
			_, err := svcWith(db, s).Signup(ctx, "fail-signup@x.test", "failsignup", "s3cr3t-pw", "")
			return err
		}},
		{name: "accept invite", run: func(s *fakesender.Sender) error {
			inviter := testutil.Must(factory.User(ctx, db))(t)
			invite := testutil.Must(factory.Invitation(ctx, db, inviter.ID, factory.Sent("fail-invitee@x.test")))(t)

			return acceptInvite(ctx, db, s, invite, "failinvitee", "s3cr3t-pw")
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			s := fakesender.New()
			s.FailWith(boom)

			require.ErrorIs(t, tc.run(s), boom)
		})
	}
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

		require.NoError(t, sendInvite(ctx, db, dbsender.NewSender(repo.New(db), fakesender.New()), inviter, "john.doe+prefix@mail.test"))
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
		queue := dbsender.NewSender(repo.New(db), fakesender.New())

		require.NoError(t, sendInvite(ctx, db, queue, inviter, "same@example.test"))
		require.Error(t, sendInvite(ctx, db, queue, inviter, " SAME@example.test "))
		require.Len(t, queued(t, db), 1)
	})

	t.Run("every invitation from one user queues its own mail", func(t *testing.T) {
		t.Parallel()

		db := testdb.New(t).DB
		inviter := newUser(t, ctx, db)
		newInvitation(t, ctx, db, inviter.ID)
		newInvitation(t, ctx, db, inviter.ID)
		queue := dbsender.NewSender(repo.New(db), fakesender.New())

		require.NoError(t, sendInvite(ctx, db, queue, inviter, "first@example.test"))
		require.NoError(t, sendInvite(ctx, db, queue, inviter, "second@example.test"))
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

		require.NoError(t, sendInvite(ctx, db, s, senderUser, "first-invitee@example.test"))

		sent := s.Sent()
		require.Len(t, sent, 1)
		require.Equal(t, "user_invitation", sent[0].EmailType)
		require.Equal(t, "first-invitee@example.test", sent[0].Mail.To[0].Address)

		// The only invitation slot senderUser had is now used, so a second
		// invite fails: it proves SendInvite marked the row as used rather
		// than leaving it free to be picked again.
		err := sendInvite(ctx, db, s, senderUser, "second-invitee@example.test")
		require.Error(t, err)
		require.Contains(t, err.Error(), "does not have any unused invites")
		require.Len(t, s.Sent(), 1, "no mail should have been sent for the rejected invite")
	})

	t.Run("fails without unused invite", func(t *testing.T) {
		t.Parallel()

		senderUser := newUser(t, ctx, db)
		s := fakesender.New()

		err := sendInvite(ctx, db, s, senderUser, "invitee@example.test")
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

		err := sendInvite(ctx, db, s, senderUser, existingUser.Email)
		require.Error(t, err)
		require.Contains(t, err.Error(), "already exists")
		require.Empty(t, s.Sent(), "no mail should have been sent for an existing email")

		// The invite slot is left untouched by the rejection: it can still be
		// used for a genuinely new email address.
		require.NoError(t, sendInvite(ctx, db, s, senderUser, "new-invitee@example.test"))
		require.Len(t, s.Sent(), 1)
	})
}

func TestCheckInviteEmail(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	t.Run("rejects existing user", func(t *testing.T) {
		t.Parallel()

		existingUser := newUser(t, ctx, db)

		for _, email := range []string{existingUser.Email, " " + strings.ToUpper(existingUser.Email) + " "} {
			err := svcWith(db, nil).CheckInviteEmail(ctx, email)
			require.Error(t, err, email)
			require.Contains(t, err.Error(), "already registered")
		}
	})

	t.Run("allows new email", func(t *testing.T) {
		t.Parallel()

		require.NoError(t, svcWith(db, nil).CheckInviteEmail(ctx, "brand-new-user@example.test"))
	})
}

func TestCheckSignupEmail_InvalidFormat(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	sender := fakesender.New()

	testCases := []string{
		"invalid",
		"@example.com",
		"user@",
		"",
	}

	for _, email := range testCases {
		t.Run(email, func(t *testing.T) {
			// Invalid email format check happens before any DB access
			msg, ok := problem(svcWith(nil, sender).CheckSignupEmail(ctx, email))
			require.False(t, ok, "email: %s", email)
			require.Equal(t, "Invalid email", msg)
		})
	}
}

func TestCheckWaitingListEmail_InvalidFormat(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	testCases := []string{
		"invalid",
		"@example.com",
		"user@",
		"",
	}

	for _, email := range testCases {
		t.Run(email, func(t *testing.T) {
			// Invalid email format check happens before any DB access
			msg, ok := problem(svcWith(nil, nil).CheckWaitingListEmail(ctx, email))
			require.False(t, ok, "email: %s", email)
			require.Equal(t, "Invalid email", msg)
		})
	}
}

func TestCheckSignupEmail(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	sender := fakesender.New()

	tests := []struct {
		name   string
		email  func(t *testing.T) string
		wantOK bool
	}{
		{"existing user", func(t *testing.T) string { return testutil.Must(factory.User(ctx, db))(t).Email }, false},
		{"plus sign", func(t *testing.T) string { return "user+test@example.com" }, false},
		{"plus sign allowed test email", func(t *testing.T) string { return "dpetroff+test@gmail.com" }, true},
		{"disposable domain", func(t *testing.T) string { return "test@mailinator.com" }, false},
		{"valid email", func(t *testing.T) string { return "valid@gmail.com" }, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, isOK := problem(svcWith(db, sender).CheckSignupEmail(ctx, tt.email(t)))
			require.Equal(t, tt.wantOK, isOK)
		})
	}
}

func TestCheckWaitingListEmail(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	tests := []struct {
		name   string
		email  func(t *testing.T) string
		wantOK bool
	}{
		{"existing user", func(t *testing.T) string { return testutil.Must(factory.User(ctx, db))(t).Email }, false},
		{"existing invitation", func(t *testing.T) string {
			inviter := testutil.Must(factory.User(ctx, db))(t)
			testutil.Must(factory.Invitation(ctx, db, inviter.ID, factory.Sent("existing@example.test")))(t)
			return "existing@example.test"
		}, false},
		{"existing invitation, spelled differently", func(t *testing.T) string {
			inviter := testutil.Must(factory.User(ctx, db))(t)
			testutil.Must(factory.Invitation(ctx, db, inviter.ID, factory.Sent("spelled@example.test")))(t)
			return " Spelled@Example.test "
		}, false},
		{"existing signup request", func(t *testing.T) string {
			return testutil.Must(factory.SignupRequest(ctx, db))(t).Email
		}, false},
		{"valid email", func(t *testing.T) string { return "valid@example.test" }, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, isOK := problem(svcWith(db, nil).CheckWaitingListEmail(ctx, tt.email(t)))
			require.Equal(t, tt.wantOK, isOK)
		})
	}
}
