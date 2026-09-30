package forms_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/can3p/pcom/pkg/forms"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/pgsession"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/ginctx"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/stretchr/testify/require"
	"github.com/volatiletech/null/v8"
)

func TestChangePasswordForm_Validate(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	tests := []struct {
		name         string
		oldPassword  string
		newPassword  string
		wantErrField string
		argon2id     bool
	}{
		{"empty old password", "", "newpassword123!", "old_password", false},
		{"empty new password", "correctpassword", "", "password", false},
		{"wrong old password", "wrongpassword", "newpassword123!", "old_password", false},
		{"weak new password", "correctpassword", "short", "password", false},
		{"success", "correctpassword", "newpassword123!", "", false},
		{"success with an argon2id hash", "correctpassword", "newpassword123!", "", true},
		{"wrong old password with an argon2id hash", "wrongpassword", "newpassword123!", "old_password", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			opts := []factory.UserOpt{factory.WithPassword("correctpassword")}
			if tt.argon2id {
				// the hash a login leaves behind (#119)
				opts = append(opts, func(u *core.User) { u.Pwdhash = null.StringFrom(pgsession.HashPassword("correctpassword")) })
			}
			user := testutil.Must(factory.User(ctx, db, opts...))(t)
			c, _ := ginctx.New(t, http.MethodPost, "/settings/change_password", nil)

			form := forms.ChangePasswordFormNew(accountsFor(db, nil), user).(*forms.ChangePasswordForm)
			form.Input.OldPassword = tt.oldPassword
			form.Input.Password = tt.newPassword

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

func TestChangePasswordForm_SaveUpdatesPassword(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	c, _ := ginctx.New(t, http.MethodPost, "/settings/change_password", nil)

	user := testutil.Must(factory.User(ctx, db, factory.WithPassword("oldpassword")))(t)

	// ChangePasswordFormNew keeps the same *core.User pointer Save mutates,
	// so the old hash is captured before Save overwrites it in place.
	oldPwdhash := user.Pwdhash

	form := forms.ChangePasswordFormNew(accountsFor(db, nil), user).(*forms.ChangePasswordForm)
	form.Input.OldPassword = "oldpassword"
	form.Input.Password = "newpassword123!"

	action, err := form.Save(c)
	require.NoError(t, err)
	require.NotNil(t, action)

	updatedUser := testutil.Must(factory.GetUser(ctx, db, user.ID))(t)
	require.NotEqual(t, oldPwdhash, updatedUser.Pwdhash)
	require.True(t, strings.HasPrefix(updatedUser.Pwdhash.String, "$argon2id$"), "a new password is stored as argon2id")

	require.NoError(t, accountsFor(db, nil).CheckCredentials(c, user.Email, "newpassword123!"))
	require.Error(t, accountsFor(db, nil).CheckCredentials(c, user.Email, "oldpassword"))
}
