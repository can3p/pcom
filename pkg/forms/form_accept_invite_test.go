package forms_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/can3p/pcom/pkg/auth"
	"github.com/can3p/pcom/pkg/forms"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/fakesender"
	"github.com/can3p/pcom/pkg/testutil/ginctx"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/gin-contrib/sessions"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
)

// newInvite creates an inviter and returns an invitation addressed to
// a unique address (pending invitations are unique per address), for tests that accept it.
func newInvite(t *testing.T, ctx context.Context, db *sqlx.DB) *core.UserInvitation {
	t.Helper()
	inviter := testutil.Must(factory.User(ctx, db))(t)
	return testutil.Must(factory.Invitation(ctx, db, inviter.ID, factory.Sent("invitee-"+uuid.NewString()+"@example.test")))(t)
}

func TestAcceptInviteForm_Validate(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	sender := fakesender.New()

	tests := []struct {
		name         string
		username     func(t *testing.T) string
		wantErrField string
	}{
		{"empty username", func(t *testing.T) string { return "" }, "username"},
		{"existing username", func(t *testing.T) string {
			return testutil.Must(factory.User(ctx, db))(t).Username
		}, "username"},
		{"invalid username format", func(t *testing.T) string { return "1abc" }, "username"},
		{"success", func(t *testing.T) string { return "newuser" }, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			invite := newInvite(t, ctx, db)
			c, _ := ginctx.New(t, http.MethodPost, "/accept_invite", nil)

			form := forms.AcceptInviteFormNew(codeAccounts(db, sender), invite).(*forms.AcceptInviteForm)
			form.Input.Username = tt.username(t)

			err := form.Validate(c)
			if tt.wantErrField == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			require.True(t, form.Errors.HasError(tt.wantErrField))
		})
	}
}

func TestAcceptInviteForm_SaveStartsTheCodeLogin(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	sender := fakesender.New()
	c, _ := ginctx.New(t, http.MethodPost, "/accept_invite", nil)

	invite := newInvite(t, ctx, db)

	form := forms.AcceptInviteFormNew(codeAccounts(db, sender), invite).(*forms.AcceptInviteForm)
	form.Input.Username = "newuser"

	_, err := form.Save(c)
	require.NoError(t, err)

	require.True(t, invite.CreatedUserID.Valid)
	require.NotEmpty(t, auth.LoginAttempt(c), "the visitor's session carries the attempt the code form finishes")
	require.Nil(t, sessions.Default(c).Get("user"), "nobody is logged in before the code is typed")
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

	form := forms.AcceptInviteFormNew(codeAccounts(db, sender), invite).(*forms.AcceptInviteForm)
	form.Input.Username = "newuser"

	_, err := form.Save(c)
	require.NoError(t, err)
	require.True(t, invite.CreatedUserID.Valid)

	newUser := testutil.Must(factory.GetUser(ctx, db, invite.CreatedUserID.String))(t)

	sendInvite := forms.SendInviteFormNew(accountsFor(db, sender), newUser).(*forms.SendInviteForm)
	sendInvite.Input.Email = "invitee-of-invitee@example.test"

	sendC, _ := ginctx.New(t, http.MethodPost, "/send_invite", nil)
	_, err = sendInvite.Save(sendC)
	require.NoError(t, err, "the new user should have received a fresh, unused invite")
}
