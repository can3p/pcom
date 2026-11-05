package accounts_test

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/can3p/pcom/pkg/jobs"
	"github.com/can3p/pcom/pkg/mail"
	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service"
	"github.com/can3p/pcom/pkg/service/accounts"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/fakequeue"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// svcWith is the accounts service over db that enqueues its jobs on q.
func svcWith(db *sql.DB, q *fakequeue.Queue) *accounts.Service {
	if q == nil {
		q = fakequeue.New()
	}

	return accounts.New(repo.New(db), nil, nil, accounts.WithCodeKey("test-key"), accounts.WithJobs(q))
}

// rendered builds the mail of every job q recorded, in order, the way the
// mail workers do when they run.
func rendered(t *testing.T, ctx context.Context, svc *accounts.Service, q *fakequeue.Queue) []*mail.Envelope {
	t.Helper()

	var out []*mail.Envelope

	for _, args := range q.Inserted() {
		var (
			env *mail.Envelope
			err error
		)

		switch a := args.(type) {
		case accounts.InvitationMailArgs:
			env, err = svc.RenderInvitation(ctx, a)
		case accounts.AdminNewUserMailArgs:
			env, err = svc.RenderAdminNewUser(ctx, a)
		case accounts.AdminThrowAwayEmailMailArgs:
			env, err = svc.RenderAdminThrowAwayEmail(ctx, a)
		case accounts.ConfirmWaitingListMailArgs:
			env, err = svc.RenderConfirmWaitingList(ctx, a)
		case accounts.AdminNewWaitingListMemberMailArgs:
			env, err = svc.RenderAdminNewWaitingListMember(ctx, a)
		case accounts.LoginCodeMailArgs:
			env, err = svc.RenderLoginCode(ctx, a)
		case accounts.ConfirmSignupMailArgs:
			env, err = svc.RenderConfirmSignup(ctx, a)
		case accounts.AdminSignupConfirmedMailArgs:
			env, err = svc.RenderAdminSignupConfirmed(ctx, a)
		case accounts.AdminPageFailureMailArgs:
			env, err = svc.RenderAdminPageFailure(ctx, a)
		default:
			t.Fatalf("unexpected job %T", args)
		}

		require.NoError(t, err, args.Kind())
		require.NotNil(t, env, args.Kind())

		out = append(out, env)
	}

	return out
}

func acceptInvite(ctx context.Context, db *sql.DB, s *fakequeue.Queue, invite *model.UserInvitation, username string) error {
	_, err := svcWith(db, s).AcceptInvite(ctx, invite, username)

	return err
}

func sendInvite(ctx context.Context, db *sql.DB, s *fakequeue.Queue, inviter *model.User, to string) error {
	return svcWith(db, s).SendInvite(ctx, inviter, to)
}

// problem splits a check's result into the reason shown to the user and
// whether the input passed. Any other error fails the test run loudly.
func problem(err error) (string, bool) {
	if invalid, ok := errors.AsType[*service.ValidationError](err); ok {
		return invalid.Message, false
	}

	if err != nil {
		panic(err)
	}

	return "", true
}

func newUser(t *testing.T, ctx context.Context, db *sql.DB) *model.User {
	t.Helper()

	return testutil.Must(factory.User(ctx, db))(t)
}

func newInvitation(t *testing.T, ctx context.Context, db *sql.DB, userID string) {
	t.Helper()

	testutil.Must(factory.Invitation(ctx, db, userID))(t)
}

func TestRegister(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	t.Run("creates an unconfirmed user without a password, tells the admin and mails the code", func(t *testing.T) {
		t.Parallel()

		q := fakequeue.New()
		svc := svcWith(db, q)

		id, err := svc.Register(ctx, "New-Signup@example.test", "newsignup", "some-campaign")
		require.NoError(t, err)
		require.NotEmpty(t, id)

		got := testutil.Must(factory.GetUserByEmail(ctx, db, "new-signup@example.test"))(t)
		require.Equal(t, "newsignup", got.Username)
		require.Nil(t, got.EmailConfirmedAt, "the email is unconfirmed until the code is typed")
		require.Nil(t, got.Pwdhash)
		require.Equal(t, "some-campaign", lo.FromPtr(got.SignupAttribution))

		require.Len(t, q.Inserted(), 2)
		require.Len(t, fakequeue.Of[accounts.AdminNewUserMailArgs](q), 1)
		require.Len(t, fakequeue.Of[accounts.ConfirmSignupMailArgs](q), 1)

		sent := rendered(t, ctx, svc, q)
		require.Len(t, sent, 2)
		require.Equal(t, "admin_new_user", sent[0].Type)
		require.Equal(t, "confirm_signup", sent[1].Type)
		require.Equal(t, "new-signup@example.test", sent[1].Mail.To[0].Address)
		require.Equal(t, id, sent[1].UniqueID)
	})

	t.Run("missing fields", func(t *testing.T) {
		t.Parallel()

		sender := fakequeue.New()

		_, err := svcWith(db, sender).Register(ctx, "", "user", "")
		require.Error(t, err)

		_, err = svcWith(db, sender).Register(ctx, "a@b.example.test", "", "")
		require.Error(t, err)

		require.Empty(t, sender.Inserted(), "a rejected signup must not notify anyone")
	})

	t.Run("a mailbox in use, under any spelling", func(t *testing.T) {
		t.Parallel()

		existing := testutil.Must(factory.User(ctx, db, factory.WithEmail("in.use@gmail.com")))(t)

		for i, email := range []string{existing.Email, "in.use+again@gmail.com", "inuse@googlemail.com"} {
			sender := fakequeue.New()

			_, err := svcWith(db, sender).Register(ctx, email, "someoneelse"+strconv.Itoa(i), "")
			msg, ok := problem(err)
			require.False(t, ok, email)
			require.Equal(t, "Email is already used in the system", msg)
			require.Empty(t, sender.Inserted(), "a failed signup must not notify anyone")
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
				if _, err := svcWith(db, fakequeue.New()).Register(ctx, email, "racer"+strconv.Itoa(i), ""); err == nil {
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

		sender := fakequeue.New()
		inviter, invite := newInvite(t, "invitee@example.test")

		svc := svcWith(db, sender)

		id, err := svc.AcceptInvite(ctx, invite, "invitee")
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

		require.Len(t, sender.Inserted(), 2)
		require.Len(t, fakequeue.Of[accounts.AdminNewUserMailArgs](sender), 1)
		require.Len(t, fakequeue.Of[accounts.LoginCodeMailArgs](sender), 1)

		sent := rendered(t, ctx, svc, sender)
		require.Len(t, sent, 2)
		require.Equal(t, "admin_new_user", sent[0].Type)
		require.Equal(t, "login_code", sent[1].Type)
		require.Equal(t, "invitee@example.test", sent[1].Mail.To[0].Address)
	})

	t.Run("an empty username consumes nothing", func(t *testing.T) {
		t.Parallel()

		sender := fakequeue.New()
		_, invite := newInvite(t, "invitee2@example.test")

		require.Error(t, acceptInvite(ctx, db, sender, invite, ""))
		require.Nil(t, invite.CreatedUserID)
		require.Empty(t, sender.Inserted())
	})

	t.Run("an address in use", func(t *testing.T) {
		t.Parallel()

		sender := fakequeue.New()
		_, invite := newInvite(t, newUser(t, ctx, db).Email)

		require.Error(t, acceptInvite(ctx, db, sender, invite, "someoneelse"), "the email column is unique")
		require.Nil(t, invite.CreatedUserID)
		require.Empty(t, sender.Inserted())
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
		run   func(s *fakequeue.Queue) (*model.UserInvitation, error)
	}{
		{name: "signup", email: "fail-signup@x.test", run: func(s *fakequeue.Queue) (*model.UserInvitation, error) {
			_, err := svcWith(db, s).Register(ctx, "fail-signup@x.test", "failsignup", "")
			return nil, err
		}},
		{name: "accept invite", email: "fail-invitee@x.test", run: func(s *fakequeue.Queue) (*model.UserInvitation, error) {
			inviter := testutil.Must(factory.User(ctx, db))(t)
			invite := testutil.Must(factory.Invitation(ctx, db, inviter.ID, factory.Sent("fail-invitee@x.test")))(t)

			return invite, acceptInvite(ctx, db, s, invite, "failinvitee")
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			s := fakequeue.New()
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
// leaves no user and no queued mail job, through the real job queue.
func TestRegisterAndAcceptInvite_AreAtomic(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	queue, err := jobs.NewInserter(db)
	require.NoError(t, err)

	svc := accounts.New(repo.New(db), nil, nil, accounts.WithJobs(queue))

	inviter := newUser(t, ctx, db)
	invite := testutil.Must(factory.Invitation(ctx, db, inviter.ID, factory.Sent("atomic-invitee@x.test")))(t)

	_, err = svc.Register(ctx, "atomic-signup@x.test", "atomicsignup", "")
	require.Error(t, err)

	_, err = svc.AcceptInvite(ctx, invite, "atomicinvitee")
	require.Error(t, err)
	require.Nil(t, invite.CreatedUserID)

	for _, email := range []string{"atomic-signup@x.test", "atomic-invitee@x.test"} {
		_, err := factory.GetUserByEmail(ctx, db, email)
		require.Error(t, err, email)
	}

	var queued int
	require.NoError(t, db.QueryRowContext(ctx, `SELECT count(*) FROM river_job WHERE kind LIKE 'mail.%'`).Scan(&queued))
	require.Zero(t, queued, "no mail is queued, so no attempt was started either")
}

// TestSendInvite_Queue checks the jobs an invitation enqueues and the mail
// each renders.
func TestSendInvite_Queue(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	queued := func(t *testing.T, svc *accounts.Service, q *fakequeue.Queue) []string {
		t.Helper()
		var to []string
		for _, e := range rendered(t, ctx, svc, q) {
			require.Equal(t, "user_invitation", e.Type)
			to = append(to, e.Mail.To[0].Address)
		}

		return to
	}

	t.Run("a plus address is invited although the plain one is registered", func(t *testing.T) {
		t.Parallel()

		db := testdb.New(t).DB
		inviter := newUser(t, ctx, db)
		newInvitation(t, ctx, db, inviter.ID)
		testutil.Must(factory.User(ctx, db, factory.WithEmail("john.doe@mail.test")))(t)
		q := fakequeue.New()

		require.NoError(t, sendInvite(ctx, db, q, inviter, "john.doe+prefix@mail.test"))
		to := queued(t, svcWith(db, q), q)
		require.Len(t, to, 1)
		require.Contains(t, to[0], "john.doe+prefix@mail.test")
	})

	t.Run("a second pending invitation to the same address is rejected", func(t *testing.T) {
		t.Parallel()

		db := testdb.New(t).DB
		inviter := newUser(t, ctx, db)
		newInvitation(t, ctx, db, inviter.ID)
		newInvitation(t, ctx, db, inviter.ID)
		queue := fakequeue.New()

		require.NoError(t, sendInvite(ctx, db, queue, inviter, "same@example.test"))
		require.Error(t, sendInvite(ctx, db, queue, inviter, " SAME@example.test "))
		require.Len(t, queued(t, svcWith(db, queue), queue), 1)
	})

	t.Run("every invitation from one user queues its own mail", func(t *testing.T) {
		t.Parallel()

		db := testdb.New(t).DB
		inviter := newUser(t, ctx, db)
		newInvitation(t, ctx, db, inviter.ID)
		newInvitation(t, ctx, db, inviter.ID)
		queue := fakequeue.New()

		require.NoError(t, sendInvite(ctx, db, queue, inviter, "first@example.test"))
		require.NoError(t, sendInvite(ctx, db, queue, inviter, "second@example.test"))

		args := fakequeue.Of[accounts.InvitationMailArgs](queue)
		require.Len(t, args, 2)
		require.NotEqual(t, args[0].InviteID, args[1].InviteID)

		to := queued(t, svcWith(db, queue), queue)
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
		s := fakequeue.New()

		require.NoError(t, sendInvite(ctx, db, s, senderUser, "first-invitee@example.test"))

		require.Len(t, fakequeue.Of[accounts.InvitationMailArgs](s), 1)

		sent := rendered(t, ctx, svcWith(db, s), s)
		require.Len(t, sent, 1)
		require.Equal(t, "user_invitation", sent[0].Type)
		require.Equal(t, "first-invitee@example.test", sent[0].Mail.To[0].Address)

		// The only invitation slot senderUser had is now used, so a second
		// invite fails: it proves SendInvite marked the row as used rather
		// than leaving it free to be picked again.
		err := sendInvite(ctx, db, s, senderUser, "second-invitee@example.test")
		require.Error(t, err)
		require.Contains(t, err.Error(), "does not have any unused invites")
		require.Len(t, s.Inserted(), 1, "no mail should have been sent for the rejected invite")
	})

	t.Run("fails without unused invite", func(t *testing.T) {
		t.Parallel()

		senderUser := newUser(t, ctx, db)
		s := fakequeue.New()

		err := sendInvite(ctx, db, s, senderUser, "invitee@example.test")
		require.Error(t, err)
		require.Contains(t, err.Error(), "does not have any unused invites")
		require.Empty(t, s.Inserted())
	})

	t.Run("rejects existing email", func(t *testing.T) {
		t.Parallel()

		senderUser := newUser(t, ctx, db)
		newInvitation(t, ctx, db, senderUser.ID)
		existingUser := newUser(t, ctx, db)
		s := fakequeue.New()

		err := sendInvite(ctx, db, s, senderUser, existingUser.Email)
		require.Error(t, err)
		require.Contains(t, err.Error(), "already exists")
		require.Empty(t, s.Inserted(), "no mail should have been sent for an existing email")

		// The invite slot is left untouched by the rejection: it can still be
		// used for a genuinely new email address.
		require.NoError(t, sendInvite(ctx, db, s, senderUser, "new-invitee@example.test"))
		require.Len(t, s.Inserted(), 1)
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
	sender := fakequeue.New()

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
	sender := fakequeue.New()

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
