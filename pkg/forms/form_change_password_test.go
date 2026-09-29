package forms_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/can3p/pcom/pkg/auth"
	"github.com/can3p/pcom/pkg/forms"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/ginctx"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/stretchr/testify/require"
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
	}{
		{"empty old password", "", "newpassword123!", "old_password"},
		{"empty new password", "correctpassword", "", "password"},
		{"wrong old password", "wrongpassword", "newpassword123!", "old_password"},
		{"weak new password", "correctpassword", "short", "password"},
		{"success", "correctpassword", "newpassword123!", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			user := testutil.Must(factory.User(ctx, db, factory.WithPassword("correctpassword")))(t)
			c, _ := ginctx.New(t, http.MethodPost, "/settings/change_password", nil)

			form := forms.ChangePasswordFormNew(user).(*forms.ChangePasswordForm)
			form.Input.OldPassword = tt.oldPassword
			form.Input.Password = tt.newPassword

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

func TestChangePasswordForm_SaveUpdatesPassword(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	c, _ := ginctx.New(t, http.MethodPost, "/settings/change_password", nil)

	user := testutil.Must(factory.User(ctx, db, factory.WithPassword("oldpassword")))(t)

	// ChangePasswordFormNew keeps the same *core.User pointer Save mutates,
	// so the old hash is captured before Save overwrites it in place.
	oldPwdhash := user.Pwdhash

	form := forms.ChangePasswordFormNew(user).(*forms.ChangePasswordForm)
	form.Input.OldPassword = "oldpassword"
	form.Input.Password = "newpassword123!"

	action, err := form.Save(c, db)
	require.NoError(t, err)
	require.NotNil(t, action)

	updatedUser := testutil.Must(factory.GetUser(ctx, db, user.ID))(t)
	require.NotEqual(t, oldPwdhash, updatedUser.Pwdhash)

	require.NoError(t, auth.CheckCredentials(c, db, user.Email, "newpassword123!"))
	require.Error(t, auth.CheckCredentials(c, db, user.Email, "oldpassword"))
}
