package accounts_test

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/can3p/pcom/pkg/mail/sender/dbsender"
	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service"
	"github.com/can3p/pcom/pkg/service/accounts"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/fakesender"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/jmoiron/sqlx"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// svcWith is the accounts service over db that sends through snd.
func svcWith(db *sqlx.DB, snd repo.MailQueue) *accounts.Service {
	return accounts.New(repo.New(db), snd, nil, accounts.WithCodeKey("test-key"))
}

func acceptInvite(ctx context.Context, db *sqlx.DB, s repo.MailQueue, invite *model.UserInvitation, username string) error {
	_, err := svcWith(db, s).AcceptInvite(ctx, invite, username)

	return err
}

func sendInvite(ctx context.Context, db *sqlx.DB, s repo.MailQueue, inviter *model.User, to string) error {
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

func newUser(t *testing.T, ctx context.Context, db *sqlx.DB) *model.User {
	t.Helper()

	return testutil.Must(factory.User(ctx, db))(t)
}

func newInvitation(t *testing.T, ctx context.Context, db *sqlx.DB, userID string) {
	t.Helper()

	testutil.Must(factory.Invitation(ctx, db, userID))(t)
}

func TestRegister(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	t.Run("creates an unconfirmed user without a password, tells the admin and mails the code", func(t *testing.T) {
		t.Parallel()

		sender := fakesender.New()

		id, err := svcWith(db, sender).Register(ctx, "New-Signup@example.test", "newsignup", "some-campaign")
		require.NoError(t, err)
		require.NotEmpty(t, id)

		got := testutil.Must(factory.GetUserByEmail(ctx, db, "new-signup@example.test"))(t)
		require.Equal(t, "newsignup", got.Username)
		require.Nil(t, got.EmailConfirmedAt, "the email is unconfirmed until the code is typed")
		require.Nil(t, got.Pwdhash)
		require.Equal(t, "some-campaign", lo.FromPtr(got.SignupAttribution))

		sent := sender.Sent()
		require.Len(t, sent, 2)
		require.Equal(t, "admin_new_user", sent[0].EmailType)
		require.Equal(t, "confirm_signup", sent[1].EmailType)
		require.Equal(t, "new-signup@example.test", sent[1].Mail.To[0].Address)
		require.Equal(t, id, sent[1].UniqueID)
	})

	t.Run("missing fields", func(t *testing.T) {
		t.Parallel()

		sender := fakesender.New()

		_, err := svcWith(db, sender).Register(ctx, "", "user", "")
		require.Error(t, err)

		_, err = svcWith(db, sender).Register(ctx, "a@b.example.test", "", "")
		require.Error(t, err)

		require.Empty(t, sender.Sent(), "a rejected signup must not notify anyone")
	})

	t.Run("a mailbox in use, under any spelling", func(t *testing.T) {
		t.Parallel()

		existing := testutil.Must(factory.User(ctx, db, factory.WithEmail("in.use@gmail.com")))(t)

		for i, email := range []string{existing.Email, "in.use+again@gmail.com", "inuse@googlemail.com"} {
			sender := fakesender.New()

			_, err := svcWith(db, sender).Register(ctx, email, "someoneelse"+strconv.Itoa(i), "")
			msg, ok := problem(err)
			require.False(t, ok, email)
			require.Equal(t, "Email is already used in the system", msg)
			require.Empty(t, sender.Sent(), "a failed signup must not notify anyone")
		}
	})

	t.Run("concurrent signups for one mailbox create one account", func(t *testing.T) {
		t.Parallel()

		var (
			wg      sync.WaitGroup
			created atomic.Int32
		)
		for i := range 10 {
			wg.Go(func() {
				email := "race+" + strconv.Itoa(i) + "@example.test"
				if _, err := svcWith(db, fakesender.New()).Register(ctx, email, "racer"+strconv.Itoa(i), ""); err == nil {
					created.Add(1)
				} else {
					var invalid *service.ValidationError
					assert.ErrorAs(t, err, &invalid, "only a taken mailbox may stop a signup")
				}
			})
		}
		wg.Wait()

		require.EqualValues(t, 1, created.Load())
	})
}

func TestAcceptInvite(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	newInvite := func(t *testing.T, to string) (*model.User, *model.UserInvitation) {
		inviter := newUser(t, ctx, db)

		return inviter, testutil.Must(factory.Invitation(ctx, db, inviter.ID, factory.Sent(to)))(t)
	}

	t.Run("creates a confirmed user without a password, connects it and mails the code", func(t *testing.T) {
		t.Parallel()

		sender := fakesender.New()
		inviter, invite := newInvite(t, "invitee@example.test")

		id, err := svcWith(db, sender).AcceptInvite(ctx, invite, "invitee")
		require.NoError(t, err)
		require.NotEmpty(t, id)

		// AcceptInvite mutates the invite it was given: CreatedUserID is set
		// to the freshly inserted user's ID.
		require.NotNil(t, invite.CreatedUserID)
		newUserID := lo.FromPtr(invite.CreatedUserID)

		got := testutil.Must(factory.GetUser(ctx, db, newUserID))(t)
		require.Equal(t, "invitee", got.Username)
		require.Equal(t, "invitee@example.test", got.Email)
		require.NotNil(t, got.EmailConfirmedAt, "accepting an invite confirms the email right away")
		require.Nil(t, got.Pwdhash)
		require.Equal(t, "accepted_invite", lo.FromPtr(got.SignupAttribution))

		require.True(t, testutil.Must(factory.ConnectionExists(ctx, db, inviter.ID, newUserID))(t))

		sent := sender.Sent()
		require.Len(t, sent, 2)
		require.Equal(t, "admin_new_user", sent[0].EmailType)
		require.Equal(t, "login_code", sent[1].EmailType)
		require.Equal(t, "invitee@example.test", sent[1].Mail.To[0].Address)
	})

	t.Run("an empty username consumes nothing", func(t *testing.T) {
		t.Parallel()

		sender := fakesender.New()
		_, invite := newInvite(t, "invitee2@example.test")

		require.Error(t, acceptInvite(ctx, db, sender, invite, ""))
		require.Nil(t, invite.CreatedUserID)
		require.Empty(t, sender.Sent())
	})

	t.Run("an address in use", func(t *testing.T) {
		t.Parallel()

		sender := fakesender.New()
		_, invite := newInvite(t, newUser(t, ctx, db).Email)

		require.Error(t, acceptInvite(ctx, db, sender, invite, "someoneelse"), "the email column is unique")
		require.Nil(t, invite.CreatedUserID)
		require.Empty(t, sender.Sent())
	})
}

func TestSignupAndAcceptInvite_ReturnAdminNotificationError(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	boom := errors.New("smtp down")

	cases := []struct {
		name  string
		email string
		run   func(s *fakesender.Sender) (*model.UserInvitation, error)
	}{
		{name: "signup", email: "fail-signup@x.test", run: func(s *fakesender.Sender) (*model.UserInvitation, error) {
			_, err := svcWith(db, s).Register(ctx, "fail-signup@x.test", "failsignup", "")
			return nil, err
		}},
		{name: "accept invite", email: "fail-invitee@x.test", run: func(s *fakesender.Sender) (*model.UserInvitation, error) {
			inviter := testutil.Must(factory.User(ctx, db))(t)
			invite := testutil.Must(factory.Invitation(ctx, db, inviter.ID, factory.Sent("fail-invitee@x.test")))(t)

			return invite, acceptInvite(ctx, db, s, invite, "failinvitee")
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			s := fakesender.New()
			s.FailWith(boom)

			invite, err := tc.run(s)
			require.ErrorIs(t, err, boom)

			_, err = factory.GetUserByEmail(ctx, db, tc.email)
			require.Error(t, err, "no user is left behind")

			if invite != nil {
				require.Nil(t, invite.CreatedUserID)
			}
		})
	}
}

// A failure after the user insert (here: no code key, so no attempt can start)
// leaves no user and no queued mail, through the real queue.
func TestRegisterAndAcceptInvite_AreAtomic(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	queue := dbsender.NewSender(repo.New(db), fakesender.New().Delivery())
	svc := accounts.New(repo.New(db), queue, nil)

	inviter := newUser(t, ctx, db)
	invite := testutil.Must(factory.Invitation(ctx, db, inviter.ID, factory.Sent("atomic-invitee@x.test")))(t)

	_, err := svc.Register(ctx, "atomic-signup@x.test", "atomicsignup", "")
	require.Error(t, err)

	_, err = svc.AcceptInvite(ctx, invite, "atomicinvitee")
	require.Error(t, err)
	require.Nil(t, invite.CreatedUserID)

	for _, email := range []string{"atomic-signup@x.test", "atomic-invitee@x.test"} {
		_, err := factory.GetUserByEmail(ctx, db, email)
		require.Error(t, err, email)
	}

	require.Empty(t, testutil.Must(factory.ListOutgoingEmails(ctx, db))(t), "no mail is queued, so no attempt was started either")
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
		for _, e := range testutil.Must(factory.ListOutgoingEmails(ctx, db, factory.EmailType("user_invitation")))(t) {
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

		require.NoError(t, sendInvite(ctx, db, dbsender.NewSender(repo.New(db), fakesender.New().Delivery()), inviter, "john.doe+prefix@mail.test"))
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
		queue := dbsender.NewSender(repo.New(db), fakesender.New().Delivery())

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
		queue := dbsender.NewSender(repo.New(db), fakesender.New().Delivery())

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

	// registered makes an account for email and returns the address to check
	registered := func(email, check string) func(t *testing.T) string {
		return func(t *testing.T) string {
			testutil.Must(factory.User(ctx, db, factory.WithEmail(email)))(t)
			return check
		}
	}
	address := func(email string) func(t *testing.T) string { return func(*testing.T) string { return email } }

	// one account per mailbox: a +tag or Gmail's dots name the same one
	tests := []struct {
		name   string
		email  func(t *testing.T) string
		wantOK bool
	}{
		{"existing user", func(t *testing.T) string { return testutil.Must(factory.User(ctx, db))(t).Email }, false},
		{"+tag of an existing user", registered("tagged@example.com", "Tagged+promo@example.com"), false},
		{"existing user with a +tag, plain", registered("plain+old@example.com", "plain@example.com"), false},
		{"new address with a +tag", address("fresh+tag@example.com"), true},
		{"Gmail dots of an existing user", registered("john.smith@gmail.com", "johnsmith+x@googlemail.com"), false},
		{"dots elsewhere are another mailbox", registered("jane.doe@example.org", "janedoe@example.org"), true},
		{"owner's test +tag beside the owner", registered("dpetroff@gmail.com", "dpetroff+test@gmail.com"), true},
		{"disposable domain", address("test@mailinator.com"), false},
		{"valid email", address("valid@gmail.com"), true},
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
