package accounts_test

import (
	"context"
	"testing"

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
	require.Len(t, fakequeue.Of[accounts.AdminPageFailureMailArgs](q), 2)

	sent := rendered(t, ctx, svc, q)
	require.Len(t, sent, 2)

	for _, e := range sent {
		require.Equal(t, "panic_notification", e.Type)
	}

	require.Contains(t, sent[0].Mail.Text, "boom report")
	require.Contains(t, sent[0].Mail.Text, user.Email)
	require.Contains(t, sent[1].Mail.Text, "visitor report")
	require.Contains(t, sent[1].Mail.Text, "Anonymous")
}

func TestRenderThrowAwayEmail(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	q := fakequeue.New()
	svc := svcWith(nil, q)

	env, err := svc.RenderAdminThrowAwayEmail(ctx, accounts.AdminThrowAwayEmailMailArgs{Email: "x@mailinator.com"})
	require.NoError(t, err)
	require.Equal(t, "throw_away_email_signup", env.Type)
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
