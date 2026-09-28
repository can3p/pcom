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
}

// TestSendInviteForm_SaveConsumesTheOnlyInviteSlot pins invite accounting:
// SendInviteForm.Save claims one of the inviter's unused invitation rows for
// the invitee. Factory has no reader for invitations, so this is confirmed
// behaviorally: with only one unused slot, a second Save for the same
// inviter has nothing left to claim and fails.
func TestSendInviteForm_SaveConsumesTheOnlyInviteSlot(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	sender := fakesender.New()

	inviter, err := factory.User(ctx, db)
	require.NoError(t, err)

	_, err = factory.Invitation(ctx, db, inviter.ID)
	require.NoError(t, err)

	c1, _ := ginctx.New(t, http.MethodPost, "/send_invite", nil)
	first := forms.SendInviteFormNew(sender, inviter).(*forms.SendInviteForm)
	first.Input.Email = "first-invitee@example.test"

	_, err = first.Save(c1, db)
	require.NoError(t, err)

	c2, _ := ginctx.New(t, http.MethodPost, "/send_invite", nil)
	second := forms.SendInviteFormNew(sender, inviter).(*forms.SendInviteForm)
	second.Input.Email = "second-invitee@example.test"

	_, err = second.Save(c2, db)
	require.Error(t, err, "the inviter has no unused invites left")
}

func TestSendInviteForm_SaveFailsWithoutAnUnusedInvite(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	sender := fakesender.New()
	c, _ := ginctx.New(t, http.MethodPost, "/send_invite", nil)

	inviter, err := factory.User(ctx, db)
	require.NoError(t, err)

	form := forms.SendInviteFormNew(sender, inviter).(*forms.SendInviteForm)
	form.Input.Email = "noinvite@example.test"

	_, err = form.Save(c, db)
	require.Error(t, err)
}
