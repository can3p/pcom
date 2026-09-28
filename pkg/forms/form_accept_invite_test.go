package forms_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/can3p/pcom/pkg/forms"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/fakesender"
	"github.com/can3p/pcom/pkg/testutil/ginctx"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/gin-contrib/sessions"
	"github.com/stretchr/testify/require"
)

func TestAcceptInviteForm_ValidateEmptyUsername(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	sender := fakesender.New()
	c, _ := ginctx.New(t, http.MethodPost, "/accept_invite", nil)

	inviter, err := factory.User(ctx, db)
	require.NoError(t, err)

	invite, err := factory.Invitation(ctx, db, inviter.ID, factory.Sent("newuser@example.test"))
	require.NoError(t, err)

	form := forms.AcceptInviteFormNew(sender, invite).(*forms.AcceptInviteForm)
	form.Input.Username = ""
	form.Input.Password = "ValidPassword123!"

	err = form.Validate(c, db)
	require.Error(t, err)
	require.True(t, form.Errors.HasError("username"))
}

func TestAcceptInviteForm_ValidateExistingUsername(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	sender := fakesender.New()
	c, _ := ginctx.New(t, http.MethodPost, "/accept_invite", nil)

	existingUser, err := factory.User(ctx, db)
	require.NoError(t, err)

	inviter, err := factory.User(ctx, db)
	require.NoError(t, err)

	invite, err := factory.Invitation(ctx, db, inviter.ID, factory.Sent("newuser@example.test"))
	require.NoError(t, err)

	form := forms.AcceptInviteFormNew(sender, invite).(*forms.AcceptInviteForm)
	form.Input.Username = existingUser.Username
	form.Input.Password = "ValidPassword123!"

	err = form.Validate(c, db)
	require.Error(t, err)
	require.True(t, form.Errors.HasError("username"))
}

func TestAcceptInviteForm_ValidateEmptyPassword(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	sender := fakesender.New()
	c, _ := ginctx.New(t, http.MethodPost, "/accept_invite", nil)

	inviter, err := factory.User(ctx, db)
	require.NoError(t, err)

	invite, err := factory.Invitation(ctx, db, inviter.ID, factory.Sent("newuser@example.test"))
	require.NoError(t, err)

	form := forms.AcceptInviteFormNew(sender, invite).(*forms.AcceptInviteForm)
	form.Input.Username = "newuser"
	form.Input.Password = ""

	err = form.Validate(c, db)
	require.Error(t, err)
	require.True(t, form.Errors.HasError("password"))
}

func TestAcceptInviteForm_ValidateInvalidPasswordFormat(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	sender := fakesender.New()
	c, _ := ginctx.New(t, http.MethodPost, "/accept_invite", nil)

	inviter, err := factory.User(ctx, db)
	require.NoError(t, err)

	invite, err := factory.Invitation(ctx, db, inviter.ID, factory.Sent("newuser@example.test"))
	require.NoError(t, err)

	form := forms.AcceptInviteFormNew(sender, invite).(*forms.AcceptInviteForm)
	form.Input.Username = "newuser"
	form.Input.Password = "short"

	err = form.Validate(c, db)
	require.Error(t, err)
	require.True(t, form.Errors.HasError("password"))
}

func TestAcceptInviteForm_ValidateInvalidUsernameFormat(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	sender := fakesender.New()
	c, _ := ginctx.New(t, http.MethodPost, "/accept_invite", nil)

	inviter, err := factory.User(ctx, db)
	require.NoError(t, err)

	invite, err := factory.Invitation(ctx, db, inviter.ID, factory.Sent("newuser@example.test"))
	require.NoError(t, err)

	form := forms.AcceptInviteFormNew(sender, invite).(*forms.AcceptInviteForm)
	form.Input.Username = "1abc"
	form.Input.Password = "ValidPassword123!"

	err = form.Validate(c, db)
	require.Error(t, err)
	require.True(t, form.Errors.HasError("username"))
}

func TestAcceptInviteForm_ValidateSuccess(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	sender := fakesender.New()
	c, _ := ginctx.New(t, http.MethodPost, "/accept_invite", nil)

	inviter, err := factory.User(ctx, db)
	require.NoError(t, err)

	invite, err := factory.Invitation(ctx, db, inviter.ID, factory.Sent("newuser@example.test"))
	require.NoError(t, err)

	form := forms.AcceptInviteFormNew(sender, invite).(*forms.AcceptInviteForm)
	form.Input.Username = "newuser"
	form.Input.Password = "ValidPassword123!"

	err = form.Validate(c, db)
	require.NoError(t, err)
}

func TestAcceptInviteForm_SaveCreatesUserAndConnection(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	sender := fakesender.New()
	c, _ := ginctx.New(t, http.MethodPost, "/accept_invite", nil)

	inviter, err := factory.User(ctx, db)
	require.NoError(t, err)

	invite, err := factory.Invitation(ctx, db, inviter.ID, factory.Sent("newuser@example.test"))
	require.NoError(t, err)

	form := forms.AcceptInviteFormNew(sender, invite).(*forms.AcceptInviteForm)
	form.Input.Username = "newuser"
	form.Input.Password = "ValidPassword123!"

	action, err := form.Save(c, db)
	require.NoError(t, err)
	require.NotNil(t, action)

	// AcceptInvite stamps f.Invite (the same *core.UserInvitation the test
	// built with the factory) with the new user's id; factory has no
	// by-email user reader, so that field is how the test gets it.
	require.True(t, invite.CreatedUserID.Valid)

	newUser, err := factory.GetUser(ctx, db, invite.CreatedUserID.String)
	require.NoError(t, err)
	require.Equal(t, "newuser", newUser.Username)

	connectionExists, err := factory.ConnectionExists(ctx, db, inviter.ID, newUser.ID)
	require.NoError(t, err)
	require.True(t, connectionExists)
}

func TestAcceptInviteForm_SaveFailsWithEmptyPassword(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	sender := fakesender.New()
	c, _ := ginctx.New(t, http.MethodPost, "/accept_invite", nil)

	inviter, err := factory.User(ctx, db)
	require.NoError(t, err)

	invite, err := factory.Invitation(ctx, db, inviter.ID, factory.Sent("newuser@example.test"))
	require.NoError(t, err)

	// Save doesn't re-run Validate, so calling it directly with an empty
	// password reaches auth.AcceptInvite's own "Not enough data" guard.
	form := forms.AcceptInviteFormNew(sender, invite).(*forms.AcceptInviteForm)
	form.Input.Username = "newuser"
	form.Input.Password = ""

	_, err = form.Save(c, db)
	require.Error(t, err)
}

func TestAcceptInviteForm_SaveLogsInUser(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	sender := fakesender.New()
	c, _ := ginctx.New(t, http.MethodPost, "/accept_invite", nil)

	inviter, err := factory.User(ctx, db)
	require.NoError(t, err)

	invite, err := factory.Invitation(ctx, db, inviter.ID, factory.Sent("newuser@example.test"))
	require.NoError(t, err)

	form := forms.AcceptInviteFormNew(sender, invite).(*forms.AcceptInviteForm)
	form.Input.Username = "newuser"
	form.Input.Password = "ValidPassword123!"

	_, err = form.Save(c, db)
	require.NoError(t, err)

	require.NotNil(t, sessions.Default(c).Get("user"), "accepting an invite must log the new user in")
}

// TestAcceptInviteForm_SaveGivesNewUserAFreshInvite exercises SendInviteForm as
// the invitee, which only succeeds when the invitee holds an unused invite:
// this is how the invite AcceptInvite grants "to make things (slowly) spread"
// is confirmed to exist without a factory reader for invitations.
func TestAcceptInviteForm_SaveGivesNewUserAFreshInvite(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	sender := fakesender.New()
	c, _ := ginctx.New(t, http.MethodPost, "/accept_invite", nil)

	inviter, err := factory.User(ctx, db)
	require.NoError(t, err)

	invite, err := factory.Invitation(ctx, db, inviter.ID, factory.Sent("newuser@example.test"))
	require.NoError(t, err)

	form := forms.AcceptInviteFormNew(sender, invite).(*forms.AcceptInviteForm)
	form.Input.Username = "newuser"
	form.Input.Password = "ValidPassword123!"

	_, err = form.Save(c, db)
	require.NoError(t, err)
	require.True(t, invite.CreatedUserID.Valid)

	newUser, err := factory.GetUser(ctx, db, invite.CreatedUserID.String)
	require.NoError(t, err)

	sendInvite := forms.SendInviteFormNew(sender, newUser).(*forms.SendInviteForm)
	sendInvite.Input.Email = "invitee-of-invitee@example.test"

	sendC, _ := ginctx.New(t, http.MethodPost, "/send_invite", nil)
	_, err = sendInvite.Save(sendC, db)
	require.NoError(t, err, "the new user should have received a fresh, unused invite")
}
