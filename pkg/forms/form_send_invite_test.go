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
	"github.com/stretchr/testify/require"
)

func TestSendInviteForm_ValidateEmptyEmail(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	sender := fakesender.New()
	c, _ := ginctx.New(t, http.MethodPost, "/send_invite", nil)

	inviter, err := factory.User(ctx, db)
	require.NoError(t, err)

	form := forms.SendInviteFormNew(sender, inviter).(*forms.SendInviteForm)
	form.Input.Email = ""

	err = form.Validate(c, db)
	require.Error(t, err)
	require.True(t, form.Errors.HasError("email"))
}

func TestSendInviteForm_ValidateInvalidFormat(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	sender := fakesender.New()
	c, _ := ginctx.New(t, http.MethodPost, "/send_invite", nil)

	inviter, err := factory.User(ctx, db)
	require.NoError(t, err)

	form := forms.SendInviteFormNew(sender, inviter).(*forms.SendInviteForm)
	form.Input.Email = "not-an-email"

	err = form.Validate(c, db)
	require.Error(t, err)
	require.True(t, form.Errors.HasError("email"))
}

func TestSendInviteForm_ValidateExistingUser(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	sender := fakesender.New()
	c, _ := ginctx.New(t, http.MethodPost, "/send_invite", nil)

	inviter, err := factory.User(ctx, db)
	require.NoError(t, err)

	existingUser, err := factory.User(ctx, db)
	require.NoError(t, err)

	form := forms.SendInviteFormNew(sender, inviter).(*forms.SendInviteForm)
	form.Input.Email = existingUser.Email

	err = form.Validate(c, db)
	require.Error(t, err)
	require.True(t, form.Errors.HasError("email"))
}

func TestSendInviteForm_ValidateSuccess(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	sender := fakesender.New()
	c, _ := ginctx.New(t, http.MethodPost, "/send_invite", nil)

	inviter, err := factory.User(ctx, db)
	require.NoError(t, err)

	// Create an unused invitation for the inviter
	_, err = factory.Invitation(ctx, db, inviter.ID)
	require.NoError(t, err)

	form := forms.SendInviteFormNew(sender, inviter).(*forms.SendInviteForm)
	form.Input.Email = "newinvitee@example.test"

	err = form.Validate(c, db)
	require.NoError(t, err)
}

func TestSendInviteForm_SaveSendsInvite(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	sender := fakesender.New()
	c, _ := ginctx.New(t, http.MethodPost, "/send_invite", nil)

	inviter, err := factory.User(ctx, db)
	require.NoError(t, err)

	// Create an unused invitation for the inviter
	_, err = factory.Invitation(ctx, db, inviter.ID)
	require.NoError(t, err)

	form := forms.SendInviteFormNew(sender, inviter).(*forms.SendInviteForm)
	form.Input.Email = "newinvitee@example.test"

	action, err := form.Save(c, db)
	require.NoError(t, err)
	require.NotNil(t, action)

	sent := sender.Sent()
	require.Len(t, sent, 1)
	require.Equal(t, "user_invitation", sent[0].EmailType)
	require.Equal(t, form.Input.Email, sent[0].Mail.To[0].Address)
}
