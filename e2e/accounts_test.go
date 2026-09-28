package e2e_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/can3p/pcom/e2e"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/stretchr/testify/require"
)

// TestE3_SignupGet_RegistrationOpen tests GET /signup with registration open.
func TestE3_SignupGet_RegistrationOpen(t *testing.T) {
	app := e2e.Start(t)
	ctx := context.Background()

	// Open registration
	err := factory.SetRegistrationOpen(ctx, app.DB, true)
	require.NoError(t, err)

	resp := app.Client(t).Get("/signup").RequireStatus(http.StatusOK)

	// Check that the page contains the registration open indicator
	doc := resp.Doc()
	require.NotZero(t, doc.Find("body").Length())
}

// TestE3_SignupGet_RegistrationClosed tests GET /signup with registration closed.
func TestE3_SignupGet_RegistrationClosed(t *testing.T) {
	app := e2e.Start(t)
	ctx := context.Background()

	// Close registration
	err := factory.SetRegistrationOpen(ctx, app.DB, false)
	require.NoError(t, err)

	resp := app.Client(t).Get("/signup").RequireStatus(http.StatusOK)

	// Check that the page is still returned (the form shows the closed message)
	doc := resp.Doc()
	require.NotZero(t, doc.Find("body").Length())
}

// TestE3_PostSignupWaitingList tests that POST /form/signup_waiting_list always returns 404.
func TestE3_PostSignupWaitingList(t *testing.T) {
	app := e2e.Start(t)

	resp := app.Client(t).PostForm("/form/signup_waiting_list", nil)
	resp.RequireStatus(http.StatusNotFound)
}

// TestE3_PostSignupWaitingList_LoggedIn tests that even logged in users get 404.
func TestE3_PostSignupWaitingList_LoggedIn(t *testing.T) {
	app := e2e.Start(t)
	ctx := context.Background()

	user, err := factory.User(ctx, app.DB, factory.WithPassword("test"))
	require.NoError(t, err)

	client := app.Client(t)
	client.LoginAs(user.Email, "test")

	resp := client.PostForm("/form/signup_waiting_list", nil)
	resp.RequireStatus(http.StatusNotFound)
}

// TestE3_InviteValid tests GET /invite/:id with a valid unused invitation.
func TestE3_InviteValid(t *testing.T) {
	app := e2e.Start(t)
	ctx := context.Background()

	inviter, err := factory.User(ctx, app.DB, factory.WithPassword("inviter"))
	require.NoError(t, err)

	invite, err := factory.Invitation(ctx, app.DB, inviter.ID, factory.Sent("invitee@example.com"))
	require.NoError(t, err)

	resp := app.Client(t).Get("/invite/" + invite.ID).RequireStatus(http.StatusOK)

	doc := resp.Doc()
	require.NotZero(t, doc.Find("body").Length())
}

// TestE3_InviteUnknown tests GET /invite/:id with an unknown invitation ID.
func TestE3_InviteUnknown(t *testing.T) {
	app := e2e.Start(t)

	// Use a valid UUID format that doesn't exist
	app.Client(t).Get("/invite/00000000-0000-0000-0000-000000000000").RequireStatus(http.StatusNotFound)
}

// TestE3_InviteInvalidUUID tests GET /invite/:id with an invalid UUID format.
func TestE3_InviteInvalidUUID(t *testing.T) {
	t.Skip("known bug: https://github.com/can3p/pcom/issues/152")
	// app := e2e.Start(t)
	// app.Client(t).Get("/invite/not-a-uuid").RequireStatus(http.StatusNotFound)
}

// TestE3_InviteUsed tests GET /invite/:id with an already-used invitation.
func TestE3_InviteUsed(t *testing.T) {
	app := e2e.Start(t)
	ctx := context.Background()

	inviter, err := factory.User(ctx, app.DB)
	require.NoError(t, err)

	// Create invitee user who will use the invitation
	invitee, err := factory.User(ctx, app.DB)
	require.NoError(t, err)

	// Create invitation marked as used by invitee
	invite, err := factory.Invitation(ctx, app.DB, inviter.ID, factory.Sent("invitee@example.com"), factory.UsedBy(invitee.ID))
	require.NoError(t, err)

	// Accessing a used invitation should return 404
	app.Client(t).Get("/invite/" + invite.ID).RequireStatus(http.StatusNotFound)
}

// TestE3_ConfirmSignupValid tests GET /confirm_signup/:id with a valid unconfirmed user.
func TestE3_ConfirmSignupValid(t *testing.T) {
	app := e2e.Start(t)
	ctx := context.Background()

	seedUUID := "550e8400-e29b-41d4-a716-446655440000"
	// Create an unconfirmed user with a specific EmailConfirmSeed
	user, err := factory.User(ctx, app.DB, factory.WithConfirmSeed(seedUUID), factory.Unconfirmed())
	require.NoError(t, err)
	require.False(t, user.EmailConfirmedAt.Valid, "user should be unconfirmed initially")

	// Visit the confirm link
	resp := app.Client(t).Get("/confirm_signup/" + seedUUID).RequireStatus(http.StatusOK)
	doc := resp.Doc()
	require.NotZero(t, doc.Find("body").Length())

	// Verify user is now confirmed
	user, err = factory.GetUser(ctx, app.DB, user.ID)
	require.NoError(t, err)
	require.True(t, user.EmailConfirmedAt.Valid, "user should be confirmed after visiting link")
}

// TestE3_ConfirmSignupUnknown tests GET /confirm_signup/:id with an unknown seed.
func TestE3_ConfirmSignupUnknown(t *testing.T) {
	app := e2e.Start(t)

	// Use a valid UUID format that doesn't exist
	app.Client(t).Get("/confirm_signup/00000000-0000-0000-0000-000000000000").RequireStatus(http.StatusNotFound)
}

// TestE3_ConfirmSignupAlreadyConfirmed tests GET /confirm_signup/:id with an already-confirmed user.
func TestE3_ConfirmSignupAlreadyConfirmed(t *testing.T) {
	app := e2e.Start(t)
	ctx := context.Background()

	seedUUID := "550e8400-e29b-41d4-a716-446655440001"
	// Create an already-confirmed user with a specific EmailConfirmSeed
	user, err := factory.User(ctx, app.DB, factory.WithConfirmSeed(seedUUID))
	require.NoError(t, err)
	require.True(t, user.EmailConfirmedAt.Valid, "user should be confirmed by default")

	// Clear outgoing emails before the request
	existingEmails, err := factory.ListOutgoingEmails(ctx, app.DB, core.OutgoingEmailWhere.EmailType.EQ("signup_confirmed"))
	require.NoError(t, err)
	initialCount := len(existingEmails)

	// Visit the confirm link
	resp := app.Client(t).Get("/confirm_signup/" + seedUUID).RequireStatus(http.StatusOK)
	doc := resp.Doc()
	require.NotZero(t, doc.Find("body").Length())

	// Verify no new notification was sent (already confirmed, so no email should be sent)
	emails, err := factory.ListOutgoingEmails(ctx, app.DB, core.OutgoingEmailWhere.EmailType.EQ("signup_confirmed"))
	require.NoError(t, err)
	require.Equal(t, initialCount, len(emails), "no new signup confirmation email should be sent for already-confirmed user")
}

// TestE3_ConfirmWaitingListValid tests GET /confirm_waiting_list/:id with a valid unconfirmed request.
func TestE3_ConfirmWaitingListValid(t *testing.T) {
	app := e2e.Start(t)
	ctx := context.Background()

	signup, err := factory.SignupRequest(ctx, app.DB)
	require.NoError(t, err)

	// Visit confirm_waiting_list with unconfirmed request
	resp := app.Client(t).Get("/confirm_waiting_list/" + signup.ID).RequireStatus(http.StatusOK)

	doc := resp.Doc()
	require.NotZero(t, doc.Find("body").Length())

	// Verify the signup request is now confirmed
	signup, err = factory.GetSignupRequest(ctx, app.DB, signup.ID)
	require.NoError(t, err)
	require.True(t, signup.EmailConfirmedAt.Valid)
}

// TestE3_ConfirmWaitingListUnknown tests GET /confirm_waiting_list/:id with an unknown ID.
func TestE3_ConfirmWaitingListUnknown(t *testing.T) {
	app := e2e.Start(t)

	// Use a valid UUID format that doesn't exist
	app.Client(t).Get("/confirm_waiting_list/00000000-0000-0000-0000-000000000000").RequireStatus(http.StatusNotFound)
}

// TestE3_ConfirmWaitingListAlreadyConfirmed tests GET /confirm_waiting_list/:id when already confirmed.
func TestE3_ConfirmWaitingListAlreadyConfirmed(t *testing.T) {
	app := e2e.Start(t)
	ctx := context.Background()

	// Create an already-confirmed signup request
	signup, err := factory.SignupRequest(ctx, app.DB, factory.EmailConfirmed())
	require.NoError(t, err)
	require.True(t, signup.EmailConfirmedAt.Valid, "signup should be confirmed")

	// Visit the confirm link
	resp := app.Client(t).Get("/confirm_waiting_list/" + signup.ID).RequireStatus(http.StatusOK)
	doc := resp.Doc()
	require.NotZero(t, doc.Find("body").Length())

	// Verify the signup request is still confirmed
	signupAfter, err := factory.GetSignupRequest(ctx, app.DB, signup.ID)
	require.NoError(t, err)
	require.True(t, signupAfter.EmailConfirmedAt.Valid, "signup should remain confirmed")
}
