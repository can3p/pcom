package e2e_test

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/can3p/pcom/e2e"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/tommy"
	"github.com/stretchr/testify/require"
)

// The account links are the one-shot URLs sent by email. Their flows through
// the browser are in e2e/browser/accounts_test.go (accepting an invite, and
// signing up, each finished with the emailed code); here are the server rules:
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

// TestAccounts_AcceptInviteStartsACodeLogin checks the wiring of the accept
// form: a free username creates the account and mails its login code, a taken
// one creates nothing.
func TestAccounts_AcceptInviteStartsACodeLogin(t *testing.T) {
	app := e2e.Start(t)
	ctx := context.Background()

	inviter, err := factory.User(ctx, app.DB)
	require.NoError(t, err)

	accept := func(t *testing.T, to, username string) *e2e.Response {
		invite, err := factory.Invitation(ctx, app.DB, inviter.ID, factory.Sent(to))
		require.NoError(t, err)

		anon := app.Client(t)
		anon.Get("/invite/" + invite.ID).RequireStatus(http.StatusOK)

		return anon.PostForm("/form/accept_invite/"+invite.ID, url.Values{"username": {username}}).RequireStatus(http.StatusOK)
	}

	t.Run("free username", func(t *testing.T) {
		to := "e2e-invitee@example.test"
		resp := accept(t, to, "e2einvitee")

		require.Equal(t, 1, resp.Doc().Find(`form[action="/form/login/code"]`).Length())

		user, err := factory.GetUserByEmail(ctx, app.DB, to)
		require.NoError(t, err)
		require.True(t, user.EmailConfirmedAt.Valid)
		require.Len(t, app.Mails(t, to, func(m tommy.Mail) bool { return m.Subject == "Your pcom login code" }), 1)
	})

	t.Run("taken username", func(t *testing.T) {
		to := "e2e-invitee-taken@example.test"
		resp := accept(t, to, inviter.Username)

		require.Equal(t, 1, resp.Doc().Find(".invalid-feedback").Length())

		_, err := factory.GetUserByEmail(ctx, app.DB, to)
		require.Error(t, err)
		app.NoMails(t, to, func(m tommy.Mail) bool { return m.Subject == "Your pcom login code" })
	})
}

// The /confirm_signup/:id link is gone: codes replaced it.
func TestAccounts_ConfirmSignupLinkIsGone(t *testing.T) {
	app := e2e.Start(t)

	app.Client(t).Get("/confirm_signup/550e8400-e29b-41d4-a716-446655440000").RequireStatus(http.StatusNotFound)
}

// TestAccounts_SignupStartsACodeLogin checks the wiring of the signup form: it
// answers with the code form, mails the code, and leaves a user without a
// password who is not confirmed yet.
func TestAccounts_SignupStartsACodeLogin(t *testing.T) {
	app := e2e.Start(t)
	ctx := context.Background()
	require.NoError(t, factory.SetRegistrationOpen(ctx, app.DB, true))

	const to = "e2e-signup@example.test"

	anon := app.Client(t)
	anon.Get("/signup").RequireStatus(http.StatusOK)

	resp := anon.PostForm("/form/signup", url.Values{"email": {to}, "username": {"e2esignup"}}).RequireStatus(http.StatusOK)
	require.Equal(t, 1, resp.Doc().Find(`form[action="/form/login/code"]`).Length())

	user, err := factory.GetUserByEmail(ctx, app.DB, to)
	require.NoError(t, err)
	require.False(t, user.EmailConfirmedAt.Valid)
	require.Len(t, app.Mails(t, to, func(m tommy.Mail) bool { return strings.Contains(m.Text, "confirmation code is") }), 1)
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
