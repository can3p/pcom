package forms_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/can3p/pcom/pkg/auth"
	"github.com/can3p/pcom/pkg/forms"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/ginctx"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/gin-contrib/sessions"
	"github.com/stretchr/testify/require"
)

func TestLoginForm_ValidateEmptyEmail(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	c, _ := ginctx.New(t, http.MethodPost, "/login", nil)

	form := forms.LoginFormNew().(*forms.LoginForm)
	form.Input.Email = ""
	form.Input.Password = "somepassword"

	err := form.Validate(c, db)
	require.Error(t, err)
	require.True(t, form.Errors.HasError("email"))
}

func TestLoginForm_ValidateEmptyPassword(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	c, _ := ginctx.New(t, http.MethodPost, "/login", nil)

	form := forms.LoginFormNew().(*forms.LoginForm)
	form.Input.Email = "valid@example.test"
	form.Input.Password = ""

	err := form.Validate(c, db)
	require.Error(t, err)
	require.True(t, form.Errors.HasError("password"))
}

func TestLoginForm_ValidateInvalidCredentials(t *testing.T) {
	t.Skip("known bug: https://github.com/can3p/pcom/issues/114")
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	ofExistingUser, err := factory.User(ctx, db, factory.WithPassword("correctpassword"))
	require.NoError(t, err)

	c, _ := ginctx.New(t, http.MethodPost, "/login", nil)

	form := forms.LoginFormNew().(*forms.LoginForm)
	form.Input.Email = ofExistingUser.Email
	form.Input.Password = "wrongpassword"

	err = form.Validate(c, db)
	require.Error(t, err)
}

func TestLoginForm_ValidateValidCredentials(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	ofExistingUser, err := factory.User(ctx, db, factory.WithPassword("correctpassword"))
	require.NoError(t, err)

	c, _ := ginctx.New(t, http.MethodPost, "/login", nil)

	form := forms.LoginFormNew().(*forms.LoginForm)
	form.Input.Email = ofExistingUser.Email
	form.Input.Password = "correctpassword"

	err = form.Validate(c, db)
	require.NoError(t, err)
}

func TestLoginForm_SaveLogsInUser(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	ofExistingUser, err := factory.User(ctx, db, factory.WithPassword("correctpassword"))
	require.NoError(t, err)

	c, _ := ginctx.New(t, http.MethodPost, "/login", nil)

	form := forms.LoginFormNew().(*forms.LoginForm)
	form.Input.Email = ofExistingUser.Email
	form.Input.Password = "correctpassword"
	form.Input.ReturnURL = ""

	action, err := form.Save(c, db)
	require.NoError(t, err)
	require.NotNil(t, action)

	require.Equal(t, ofExistingUser.ID, sessions.Default(c).Get("user"))
}

func TestLoginForm_SaveFailsWithWrongPassword(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	ofExistingUser, err := factory.User(ctx, db, factory.WithPassword("correctpassword"))
	require.NoError(t, err)

	c, _ := ginctx.New(t, http.MethodPost, "/login", nil)

	// Save doesn't re-run Validate, so calling it directly with the wrong
	// password reaches auth.Login's own bad-credentials error path.
	form := forms.LoginFormNew().(*forms.LoginForm)
	form.Input.Email = ofExistingUser.Email
	form.Input.Password = "wrongpassword"

	_, err = form.Save(c, db)
	require.Error(t, err)
}

func TestLoginForm_SaveRedirectsToSignedReturnURL(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	ofExistingUser, err := factory.User(ctx, db, factory.WithPassword("correctpassword"))
	require.NoError(t, err)

	c, _ := ginctx.New(t, http.MethodPost, "/login", nil)

	form := forms.LoginFormNew().(*forms.LoginForm)
	form.Input.Email = ofExistingUser.Email
	form.Input.Password = "correctpassword"
	form.Input.ReturnURL = "/feed"
	form.Input.Sign = auth.HashValue(form.Input.ReturnURL)

	action, err := form.Save(c, db)
	require.NoError(t, err)
	require.NotNil(t, action)
}
