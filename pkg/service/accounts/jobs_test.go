package accounts_test

import (
	"context"
	"testing"

	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service/accounts"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/fakequeue"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/stretchr/testify/require"
)

func TestSendAdminMail_PageFailure(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := testdb.New(t).DB
	user := testutil.Must(factory.User(ctx, db))(t)

	q := fakequeue.New()
	svc := svcWith(db, q)

	require.NoError(t, svc.SendAdminMail(ctx, "boom report", user))
	require.NoError(t, svc.SendAdminMail(ctx, "visitor report", nil))

	sent := rendered(t, ctx, svc, q)
	require.Len(t, sent, 2)

	for _, e := range sent {
		require.Equal(t, "panic_notification", e.Type)
		require.Equal(t, testAdmin, e.Mail.To[0].Address)
		require.Equal(t, "Panic on the page", e.Mail.Subject)
	}

	require.Contains(t, sent[0].Mail.Text, "boom report")
	require.Contains(t, sent[0].Mail.Text, user.Email)
	require.Contains(t, sent[1].Mail.Text, "visitor report")
	require.Contains(t, sent[1].Mail.Text, "Anonymous")
}

func TestRenderThrowAwayEmail(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	svc := svcWith(nil, nil)

	env, err := svc.RenderAdminThrowAwayEmail(ctx, accounts.AdminThrowAwayEmailMailArgs{Email: "x@mailinator.com"})
	require.NoError(t, err)
	require.Equal(t, "throw_away_email_signup", env.Type)
	require.Equal(t, testAdmin, env.Mail.To[0].Address)
	require.Equal(t, "An attempt to use a throwaway email domain on pcom", env.Mail.Subject)
	require.Contains(t, env.Mail.Text, "x@mailinator.com")
}

func TestRenderMails_RowsGoneSendNothing(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := testdb.New(t).DB
	svc := svcWith(db, nil)
	missing := "00000000-0000-0000-0000-000000000000"

	for name, render := range map[string]func() (any, error){
		"invitation": func() (any, error) {
			return svc.RenderInvitation(ctx, accounts.InvitationMailArgs{InviteID: missing, To: "a@b.test"})
		},
		"admin new user": func() (any, error) {
			return svc.RenderAdminNewUser(ctx, accounts.AdminNewUserMailArgs{UserID: missing})
		},
		"confirm waiting list": func() (any, error) {
			return svc.RenderConfirmWaitingList(ctx, accounts.ConfirmWaitingListMailArgs{RequestID: missing})
		},
		"admin waiting list": func() (any, error) {
			return svc.RenderAdminNewWaitingListMember(ctx, accounts.AdminNewWaitingListMemberMailArgs{RequestID: missing})
		},
		"login code": func() (any, error) {
			return svc.RenderLoginCode(ctx, accounts.LoginCodeMailArgs{AttemptID: missing, UserID: missing, Code: "1"})
		},
		"confirm signup": func() (any, error) {
			return svc.RenderConfirmSignup(ctx, accounts.ConfirmSignupMailArgs{AttemptID: missing, UserID: missing, Code: "1"})
		},
		"signup confirmed": func() (any, error) {
			return svc.RenderAdminSignupConfirmed(ctx, accounts.AdminSignupConfirmedMailArgs{UserID: missing})
		},
		"page failure": func() (any, error) {
			return svc.RenderAdminPageFailure(ctx, accounts.AdminPageFailureMailArgs{Report: "r", UserID: missing})
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			env, err := render()
			require.NoError(t, err)
			require.Nil(t, env)
		})
	}
}

// Every kind RegisterJobs registers renders the mail it names, to its
// recipient, with the subject and a fact of the rows it was enqueued for.
func TestRenderMails_EveryKind(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := testdb.New(t).DB
	svc := svcWith(db, nil)

	user := testutil.Must(factory.User(ctx, db))(t)
	inviter := testutil.Must(factory.User(ctx, db))(t)
	invite := testutil.Must(factory.Invitation(ctx, db, inviter.ID, factory.Sent("invitee@example.test")))(t)
	request := testutil.Must(factory.SignupRequest(ctx, db))(t)

	tests := []struct {
		args    repo.JobArgs
		emailTp string
		to      string
		subject string
		body    string
	}{
		{accounts.InvitationMailArgs{InviteID: invite.ID, To: "invitee@example.test"}, "user_invitation", "invitee@example.test", "Welcome to pcom", "/" + invite.ID},
		{accounts.AdminNewUserMailArgs{UserID: user.ID}, "admin_new_user", testAdmin, "New User on pcom", user.Email},
		{accounts.AdminThrowAwayEmailMailArgs{Email: "x@mailinator.com"}, "throw_away_email_signup", testAdmin, "An attempt to use a throwaway email domain on pcom", "x@mailinator.com"},
		{accounts.ConfirmWaitingListMailArgs{RequestID: request.ID}, "waiting_list_confirm", request.Email, "Waiting list on pcom", "/" + request.ID},
		{accounts.AdminNewWaitingListMemberMailArgs{RequestID: request.ID}, "new_waiting_list_member", testAdmin, "New waiting list member on pcom", request.Email},
		{accounts.LoginCodeMailArgs{AttemptID: "attempt-1", UserID: user.ID, Code: "12345678"}, "login_code", user.Email, "Your pcom login code", "12345678"},
		{accounts.ConfirmSignupMailArgs{AttemptID: "attempt-2", UserID: user.ID, Code: "87654321"}, "confirm_signup", user.Email, "Welcome to pcom", "87654321"},
		{accounts.AdminSignupConfirmedMailArgs{UserID: user.ID}, "signup_confirmed", testAdmin, "New User confirmed email on pcom", user.Email},
		{accounts.AdminPageFailureMailArgs{Report: "a report", UserID: user.ID}, "panic_notification", testAdmin, "Panic on the page", "a report"},
	}

	for _, tt := range tests {
		t.Run(tt.args.Kind(), func(t *testing.T) {
			t.Parallel()

			require.Equal(t, "mail."+tt.emailTp, tt.args.Kind())

			q := fakequeue.New()
			require.NoError(t, q.Insert(ctx, nil, tt.args))

			env := rendered(t, ctx, svc, q)[0]
			require.Equal(t, tt.emailTp, env.Type)
			require.NotEmpty(t, env.UniqueID)
			require.Len(t, env.Mail.To, 1)
			require.Equal(t, tt.to, env.Mail.To[0].Address)
			require.Equal(t, tt.subject, env.Mail.Subject)
			require.Contains(t, env.Mail.Text, tt.body)
		})
	}
}
