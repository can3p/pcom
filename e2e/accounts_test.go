package e2e_test

import (
	"context"
	"net/http"
	"net/url"
	"testing"

	"github.com/can3p/pcom/e2e"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/stretchr/testify/require"
)

// The account links are the one-shot URLs sent by email. Their flows through
// the browser are in e2e/browser/accounts_test.go (accepting an invite, and
// signing up and confirming, skipped for #139); here are the server rules:
// which link is honored, which is refused, and what the database records.

// TestAccounts_SignupRendersFormForRegistrationState checks that /signup offers
// the signup form only while registration is open, and the waiting list
// otherwise.
func TestAccounts_SignupRendersFormForRegistrationState(t *testing.T) {
	app := e2e.Start(t)
	ctx := context.Background()

	for _, tc := range []struct {
		open           bool
		want, notWant  string
		wantHeaderText string
	}{
		{true, `form[action="/form/signup"]`, `form[action="/form/signup_waiting_list"]`, "New Account"},
		{false, `form[action="/form/signup_waiting_list"]`, `form[action="/form/signup"]`, "Join the waiting list"},
	} {
		require.NoError(t, factory.SetRegistrationOpen(ctx, app.DB, tc.open))

		doc := app.Client(t).Get("/signup").RequireStatus(http.StatusOK).Doc()
		require.Equal(t, 1, doc.Find(tc.want).Length(), "registration open=%v", tc.open)
		require.Zero(t, doc.Find(tc.notWant).Length(), "registration open=%v", tc.open)
		require.Contains(t, doc.Find(".card-header").Text(), tc.wantHeaderText, "registration open=%v", tc.open)
	}
}

// TestAccounts_SignupWaitingListIsDisabled pins the waiting-list endpoint as off:
// bots abused it, and it stays a 404 until signups come back with bot
// protection (https://github.com/can3p/pcom/issues/123).
func TestAccounts_SignupWaitingListIsDisabled(t *testing.T) {
	app := e2e.Start(t)

	app.Client(t).PostForm("/form/signup_waiting_list", nil).RequireStatus(http.StatusNotFound)
}

// TestAccounts_InviteLinks checks that /invite/:id serves the accept form for an
// unused invitation only.
func TestAccounts_InviteLinks(t *testing.T) {
	app := e2e.Start(t)
	ctx := context.Background()

	inviter, err := factory.User(ctx, app.DB)
	require.NoError(t, err)

	t.Run("unused", func(t *testing.T) {
		invite, err := factory.Invitation(ctx, app.DB, inviter.ID, factory.Sent("invitee@example.test"))
		require.NoError(t, err)

		doc := app.Client(t).Get("/invite/" + invite.ID).RequireStatus(http.StatusOK).Doc()
		require.Equal(t, 1, doc.Find(`form[action="/form/accept_invite/`+invite.ID+`"]`).Length())
		require.Contains(t, doc.Find(".card-header").Text(), inviter.Username)
	})

	t.Run("used", func(t *testing.T) {
		invitee, err := factory.User(ctx, app.DB)
		require.NoError(t, err)

		invite, err := factory.Invitation(ctx, app.DB, inviter.ID, factory.Sent("used@example.test"), factory.UsedBy(invitee.ID))
		require.NoError(t, err)

		app.Client(t).Get("/invite/" + invite.ID).RequireStatus(http.StatusNotFound)
	})

	t.Run("unknown", func(t *testing.T) {
		app.Client(t).Get("/invite/00000000-0000-0000-0000-000000000000").RequireStatus(http.StatusNotFound)
	})
}

// TestAccounts_MalformedUUIDPathParam: a path id that is not a UUID is a 404,
// not a 500 from the database (#152).
func TestAccounts_MalformedUUIDPathParam(t *testing.T) {
	t.Parallel()

	app := e2e.Start(t)
	_, client := newLoggedIn(t, app)

	for _, path := range []string{
		"/shared/not-a-uuid",
		"/posts/not-a-uuid",
		"/posts/not-a-uuid/md",
		"/posts/not-a-uuid/zip",
		"/posts/not-a-uuid/edit",
	} {
		t.Run(path, func(t *testing.T) {
			client.Get(path).RequireStatus(http.StatusNotFound)
		})
	}

	// guest-only routes redirect a logged-in user, so use anonymous clients
	for _, path := range []string{
		"/invite/not-a-uuid",
		"/confirm_signup/not-a-uuid",
		"/confirm_waiting_list/not-a-uuid",
	} {
		t.Run(path, func(t *testing.T) {
			app.Client(t).Get(path).RequireStatus(http.StatusNotFound)
		})
	}

	t.Run("/rss/private/not-a-uuid", func(t *testing.T) {
		app.Client(t).Get("/rss/private/not-a-uuid").RequireStatus(http.StatusNotFound)
	})

	t.Run("POST /form/accept_invite/not-a-uuid", func(t *testing.T) {
		anon := app.Client(t)
		anon.Get("/login").RequireStatus(http.StatusOK) // picks up the CSRF token
		anon.PostForm("/form/accept_invite/not-a-uuid", url.Values{}).RequireStatus(http.StatusNotFound)
	})
}

// TestAccounts_ConfirmSignup checks that /confirm_signup/:seed confirms the account
// and notifies the admins once, and only for a known seed.
func TestAccounts_ConfirmSignup(t *testing.T) {
	app := e2e.Start(t)
	ctx := context.Background()

	adminMails := func(t *testing.T) int {
		t.Helper()

		emails, err := factory.ListOutgoingEmails(ctx, app.DB, core.OutgoingEmailWhere.EmailType.EQ("signup_confirmed"))
		require.NoError(t, err)

		return len(emails)
	}

	t.Run("unconfirmed", func(t *testing.T) {
		seed := "550e8400-e29b-41d4-a716-446655440000"
		user, err := factory.User(ctx, app.DB, factory.WithConfirmSeed(seed), factory.Unconfirmed())
		require.NoError(t, err)

		before := adminMails(t)
		app.Client(t).Get("/confirm_signup/" + seed).RequireStatus(http.StatusOK)

		user, err = factory.GetUser(ctx, app.DB, user.ID)
		require.NoError(t, err)
		require.True(t, user.EmailConfirmedAt.Valid)
		require.Equal(t, before+1, adminMails(t))
	})

	t.Run("already confirmed", func(t *testing.T) {
		seed := "550e8400-e29b-41d4-a716-446655440001"
		user, err := factory.User(ctx, app.DB, factory.WithConfirmSeed(seed))
		require.NoError(t, err)
		user, err = factory.GetUser(ctx, app.DB, user.ID) // the stored, microsecond precision time
		require.NoError(t, err)

		before := adminMails(t)
		app.Client(t).Get("/confirm_signup/" + seed).RequireStatus(http.StatusOK)

		after, err := factory.GetUser(ctx, app.DB, user.ID)
		require.NoError(t, err)
		require.True(t, user.EmailConfirmedAt.Time.Equal(after.EmailConfirmedAt.Time), "the confirmation time must not move")
		require.Equal(t, before, adminMails(t), "no second admin notification")
	})

	t.Run("unknown", func(t *testing.T) {
		app.Client(t).Get("/confirm_signup/00000000-0000-0000-0000-000000000000").RequireStatus(http.StatusNotFound)
	})
}

// TestAccounts_ConfirmWaitingList checks that /confirm_waiting_list/:id records the
// confirmation once, and only for a known request.
func TestAccounts_ConfirmWaitingList(t *testing.T) {
	app := e2e.Start(t)
	ctx := context.Background()

	t.Run("unconfirmed", func(t *testing.T) {
		signup, err := factory.SignupRequest(ctx, app.DB)
		require.NoError(t, err)

		app.Client(t).Get("/confirm_waiting_list/" + signup.ID).RequireStatus(http.StatusOK)

		signup, err = factory.GetSignupRequest(ctx, app.DB, signup.ID)
		require.NoError(t, err)
		require.True(t, signup.EmailConfirmedAt.Valid)
	})

	t.Run("already confirmed", func(t *testing.T) {
		signup, err := factory.SignupRequest(ctx, app.DB, factory.EmailConfirmed())
		require.NoError(t, err)
		signup, err = factory.GetSignupRequest(ctx, app.DB, signup.ID) // the stored, microsecond precision time
		require.NoError(t, err)

		app.Client(t).Get("/confirm_waiting_list/" + signup.ID).RequireStatus(http.StatusOK)

		after, err := factory.GetSignupRequest(ctx, app.DB, signup.ID)
		require.NoError(t, err)
		require.True(t, signup.EmailConfirmedAt.Time.Equal(after.EmailConfirmedAt.Time), "the confirmation time must not move")
	})

	t.Run("unknown", func(t *testing.T) {
		app.Client(t).Get("/confirm_waiting_list/00000000-0000-0000-0000-000000000000").RequireStatus(http.StatusNotFound)
	})
}
