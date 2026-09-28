package forms_test

import (
	"context"
	"net/http"
	"testing"

	"strings"

	"github.com/can3p/pcom/pkg/auth"
	"github.com/can3p/pcom/pkg/forms"
	"github.com/can3p/pcom/pkg/links"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/ginctx"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/can3p/pcom/pkg/util"
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
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	existingUser, err := factory.User(ctx, db, factory.WithPassword("correctpassword"))
	require.NoError(t, err)

	c, _ := ginctx.New(t, http.MethodPost, "/login", nil)

	form := forms.LoginFormNew().(*forms.LoginForm)
	form.Input.Email = existingUser.Email
	form.Input.Password = "wrongpassword"

	err = form.Validate(c, db)
	require.Error(t, err)
}

func TestLoginForm_ValidateCaseInsensitiveEmail(t *testing.T) {
	t.Skip("known bug #114: login fails when the email's case doesn't match the stored one")
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	existingUser, err := factory.User(ctx, db, factory.WithPassword("correctpassword"))
	require.NoError(t, err)

	c, _ := ginctx.New(t, http.MethodPost, "/login", nil)

	form := forms.LoginFormNew().(*forms.LoginForm)
	form.Input.Email = strings.ToUpper(existingUser.Email)
	form.Input.Password = "correctpassword"

	err = form.Validate(c, db)
	require.NoError(t, err, "login should succeed regardless of the email's case")
}

func TestLoginForm_ValidateValidCredentials(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	existingUser, err := factory.User(ctx, db, factory.WithPassword("correctpassword"))
	require.NoError(t, err)

	c, _ := ginctx.New(t, http.MethodPost, "/login", nil)

	form := forms.LoginFormNew().(*forms.LoginForm)
	form.Input.Email = existingUser.Email
	form.Input.Password = "correctpassword"

	err = form.Validate(c, db)
	require.NoError(t, err)
}

func TestLoginForm_SaveRedirectsToSignedReturnURL(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	existingUser, err := factory.User(ctx, db, factory.WithPassword("correctpassword"))
	require.NoError(t, err)

	c, w := ginctx.New(t, http.MethodPost, "/login", nil)

	form := forms.LoginFormNew().(*forms.LoginForm)
	form.Input.Email = existingUser.Email
	form.Input.Password = "correctpassword"
	form.Input.ReturnURL = "/feed"
	form.Input.Sign = auth.HashValue(form.Input.ReturnURL)

	action, err := form.Save(c, db)
	require.NoError(t, err)
	action(c, form)

	require.Equal(t, util.SiteRoot()+"/feed", w.Header().Get("HX-Redirect"))
}

func TestLoginForm_SaveRedirectsHomeWithBadSignature(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	existingUser, err := factory.User(ctx, db, factory.WithPassword("correctpassword"))
	require.NoError(t, err)

	c, w := ginctx.New(t, http.MethodPost, "/login", nil)

	form := forms.LoginFormNew().(*forms.LoginForm)
	form.Input.Email = existingUser.Email
	form.Input.Password = "correctpassword"
	form.Input.ReturnURL = "/feed"
	form.Input.Sign = "not-a-valid-signature"

	action, err := form.Save(c, db)
	require.NoError(t, err)
	action(c, form)

	require.Equal(t, links.DefaultAuthorizedHome(), w.Header().Get("HX-Redirect"))
}
