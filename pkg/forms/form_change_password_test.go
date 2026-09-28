package forms_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/can3p/pcom/pkg/forms"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/ginctx"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/stretchr/testify/require"
)

func TestChangePasswordForm_ValidateEmptyOldPassword(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	c, _ := ginctx.New(t, http.MethodPost, "/settings/change_password", nil)

	ofUser, err := factory.User(ctx, db, factory.WithPassword("oldpassword"))
	require.NoError(t, err)

	form := forms.ChangePasswordFormNew(ofUser).(*forms.ChangePasswordForm)
	form.Input.OldPassword = ""
	form.Input.Password = "newpassword123!"

	err = form.Validate(c, db)
	require.Error(t, err)
	require.True(t, form.Errors.HasError("old_password"))
}

func TestChangePasswordForm_ValidateEmptyNewPassword(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	c, _ := ginctx.New(t, http.MethodPost, "/settings/change_password", nil)

	ofUser, err := factory.User(ctx, db, factory.WithPassword("oldpassword"))
	require.NoError(t, err)

	form := forms.ChangePasswordFormNew(ofUser).(*forms.ChangePasswordForm)
	form.Input.OldPassword = "oldpassword"
	form.Input.Password = ""

	err = form.Validate(c, db)
	require.Error(t, err)
	require.True(t, form.Errors.HasError("password"))
}

func TestChangePasswordForm_ValidateWrongOldPassword(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	c, _ := ginctx.New(t, http.MethodPost, "/settings/change_password", nil)

	ofUser, err := factory.User(ctx, db, factory.WithPassword("correctpassword"))
	require.NoError(t, err)

	form := forms.ChangePasswordFormNew(ofUser).(*forms.ChangePasswordForm)
	form.Input.OldPassword = "wrongpassword"
	form.Input.Password = "newpassword123!"

	err = form.Validate(c, db)
	require.Error(t, err)
	require.True(t, form.Errors.HasError("old_password"))
}

func TestChangePasswordForm_ValidateWeakNewPassword(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	c, _ := ginctx.New(t, http.MethodPost, "/settings/change_password", nil)

	ofUser, err := factory.User(ctx, db, factory.WithPassword("correctpassword"))
	require.NoError(t, err)

	form := forms.ChangePasswordFormNew(ofUser).(*forms.ChangePasswordForm)
	form.Input.OldPassword = "correctpassword"
	form.Input.Password = "short"

	err = form.Validate(c, db)
	require.Error(t, err)
	require.True(t, form.Errors.HasError("password"))
}

func TestChangePasswordForm_ValidateSuccess(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	c, _ := ginctx.New(t, http.MethodPost, "/settings/change_password", nil)

	ofUser, err := factory.User(ctx, db, factory.WithPassword("correctpassword"))
	require.NoError(t, err)

	form := forms.ChangePasswordFormNew(ofUser).(*forms.ChangePasswordForm)
	form.Input.OldPassword = "correctpassword"
	form.Input.Password = "newpassword123!"

	err = form.Validate(c, db)
	require.NoError(t, err)
}

func TestChangePasswordForm_SaveUpdatesPassword(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	c, _ := ginctx.New(t, http.MethodPost, "/settings/change_password", nil)

	ofUser, err := factory.User(ctx, db, factory.WithPassword("oldpassword"))
	require.NoError(t, err)

	// ChangePasswordFormNew keeps the same *core.User pointer Save mutates,
	// so the old hash is captured before Save overwrites it in place.
	ofOldPwdhash := ofUser.Pwdhash

	form := forms.ChangePasswordFormNew(ofUser).(*forms.ChangePasswordForm)
	form.Input.OldPassword = "oldpassword"
	form.Input.Password = "newpassword123!"

	action, err := form.Save(c, db)
	require.NoError(t, err)
	require.NotNil(t, action)

	ofUpdatedUser, err := factory.GetUser(ctx, db, ofUser.ID)
	require.NoError(t, err)
	require.NotEqual(t, ofOldPwdhash, ofUpdatedUser.Pwdhash)
}
