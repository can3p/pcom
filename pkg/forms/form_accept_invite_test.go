package forms_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/can3p/pcom/pkg/forms"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/fakesender"
	"github.com/can3p/pcom/pkg/testutil/ginctx"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/gin-contrib/sessions"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
)

// newInvite creates an inviter and returns an invitation addressed to
// newuser@example.test, for tests that accept it.
func newInvite(t *testing.T, ctx context.Context, db *sqlx.DB) *core.UserInvitation {
	t.Helper()
	inviter := testutil.Must(factory.User(ctx, db))(t)
	return testutil.Must(factory.Invitation(ctx, db, inviter.ID, factory.Sent("newuser@example.test")))(t)
}

func TestAcceptInviteForm_Validate(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	sender := fakesender.New()

	tests := []struct {
		name         string
		username     func(t *testing.T) string
		password     string
		wantErrField string
	}{
		{"empty username", func(t *testing.T) string { return "" }, "ValidPassword123!", "username"},
		{"existing username", func(t *testing.T) string {
			return testutil.Must(factory.User(ctx, db))(t).Username
		}, "ValidPassword123!", "username"},
		{"empty password", func(t *testing.T) string { return "newuser" }, "", "password"},
		{"invalid password format", func(t *testing.T) string { return "newuser" }, "short", "password"},
		{"invalid username format", func(t *testing.T) string { return "1abc" }, "ValidPassword123!", "username"},
		{"success", func(t *testing.T) string { return "newuser" }, "ValidPassword123!", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			invite := newInvite(t, ctx, db)
			c, _ := ginctx.New(t, http.MethodPost, "/accept_invite", nil)

			form := forms.AcceptInviteFormNew(sender, invite).(*forms.AcceptInviteForm)
			form.Input.Username = tt.username(t)
			form.Input.Password = tt.password

			err := form.Validate(c, db)
			if tt.wantErrField == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			require.True(t, form.Errors.HasError(tt.wantErrField))
		})
	}
}

func TestAcceptInviteForm_SaveLogsInUser(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	sender := fakesender.New()
	c, _ := ginctx.New(t, http.MethodPost, "/accept_invite", nil)

	invite := newInvite(t, ctx, db)

	form := forms.AcceptInviteFormNew(sender, invite).(*forms.AcceptInviteForm)
	form.Input.Username = "newuser"
	form.Input.Password = "ValidPassword123!"

	_, err := form.Save(c, db)
	require.NoError(t, err)

	require.True(t, invite.CreatedUserID.Valid)
	require.Equal(t, invite.CreatedUserID.String, sessions.Default(c).Get("user"),
		"accepting an invite must log the newly created user in")
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

	invite := newInvite(t, ctx, db)

	form := forms.AcceptInviteFormNew(sender, invite).(*forms.AcceptInviteForm)
	form.Input.Username = "newuser"
	form.Input.Password = "ValidPassword123!"

	_, err := form.Save(c, db)
	require.NoError(t, err)
	require.True(t, invite.CreatedUserID.Valid)

	newUser := testutil.Must(factory.GetUser(ctx, db, invite.CreatedUserID.String))(t)

	sendInvite := forms.SendInviteFormNew(sender, newUser).(*forms.SendInviteForm)
	sendInvite.Input.Email = "invitee-of-invitee@example.test"

	sendC, _ := ginctx.New(t, http.MethodPost, "/send_invite", nil)
	_, err = sendInvite.Save(sendC, db)
	require.NoError(t, err, "the new user should have received a fresh, unused invite")
}
