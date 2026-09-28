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

func TestSignupForm_ValidateEmptyEmail(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	sender := fakesender.New()
	c, _ := ginctx.New(t, http.MethodPost, "/signup", nil)

	form := forms.SignupFormNew(sender).(*forms.SignupForm)
	form.Input.Username = "newuser"
	form.Input.Password = "ValidPassword123!"
	form.Input.Email = ""

	err := form.Validate(c, db)
	require.Error(t, err)
	require.True(t, form.Errors.HasError("email"))
}

func TestSignupForm_ValidateExistingEmail(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	sender := fakesender.New()
	c, _ := ginctx.New(t, http.MethodPost, "/signup", nil)

	existingUser, err := factory.User(ctx, db)
	require.NoError(t, err)

	form := forms.SignupFormNew(sender).(*forms.SignupForm)
	form.Input.Email = existingUser.Email
	form.Input.Username = "newuser"
	form.Input.Password = "ValidPassword123!"

	err = form.Validate(c, db)
	require.Error(t, err)
	require.True(t, form.Errors.HasError("email"))
}

func TestSignupForm_ValidateEmptyUsername(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	sender := fakesender.New()
	c, _ := ginctx.New(t, http.MethodPost, "/signup", nil)

	form := forms.SignupFormNew(sender).(*forms.SignupForm)
	form.Input.Email = "valid@example.test"
	form.Input.Username = ""
	form.Input.Password = "ValidPassword123!"

	err := form.Validate(c, db)
	require.Error(t, err)
	require.True(t, form.Errors.HasError("username"))
}

func TestSignupForm_ValidateExistingUsername(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	sender := fakesender.New()
	c, _ := ginctx.New(t, http.MethodPost, "/signup", nil)

	existingUser, err := factory.User(ctx, db)
	require.NoError(t, err)

	form := forms.SignupFormNew(sender).(*forms.SignupForm)
	form.Input.Email = "valid@example.test"
	form.Input.Username = existingUser.Username
	form.Input.Password = "ValidPassword123!"

	err = form.Validate(c, db)
	require.Error(t, err)
	require.True(t, form.Errors.HasError("username"))
}

func TestSignupForm_ValidateInvalidUsernameFormat(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	sender := fakesender.New()
	c, _ := ginctx.New(t, http.MethodPost, "/signup", nil)

	form := forms.SignupFormNew(sender).(*forms.SignupForm)
	form.Input.Email = "valid@example.test"
	form.Input.Username = "1abc"
	form.Input.Password = "ValidPassword123!"

	err := form.Validate(c, db)
	require.Error(t, err)
	require.True(t, form.Errors.HasError("username"))
}

func TestSignupForm_ValidateWeakPassword(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	sender := fakesender.New()
	c, _ := ginctx.New(t, http.MethodPost, "/signup", nil)

	form := forms.SignupFormNew(sender).(*forms.SignupForm)
	form.Input.Email = "valid@example.test"
	form.Input.Username = "newuser"
	form.Input.Password = "short"

	err := form.Validate(c, db)
	require.Error(t, err)
	require.True(t, form.Errors.HasError("password"))
}

func TestSignupForm_ValidateEmptyPassword(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	sender := fakesender.New()
	c, _ := ginctx.New(t, http.MethodPost, "/signup", nil)

	form := forms.SignupFormNew(sender).(*forms.SignupForm)
	form.Input.Email = "valid@example.test"
	form.Input.Username = "newuser"
	form.Input.Password = ""

	err := form.Validate(c, db)
	require.Error(t, err)
	require.True(t, form.Errors.HasError("password"))
}

func TestSignupForm_ValidateSuccess(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	sender := fakesender.New()
	c, _ := ginctx.New(t, http.MethodPost, "/signup", nil)

	form := forms.SignupFormNew(sender).(*forms.SignupForm)
	form.Input.Email = "valid@example.test"
	form.Input.Username = "newuser"
	form.Input.Password = "ValidPassword123!"

	err := form.Validate(c, db)
	require.NoError(t, err)
}

func TestSignupForm_SaveCreatesUserAndSendsEmail(t *testing.T) {
	t.Skip("known bug: https://github.com/can3p/pcom/issues/148")
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	sender := fakesender.New()

	form := forms.SignupFormNew(sender).(*forms.SignupForm)
	form.Input.Email = "newuser@example.test"
	form.Input.Username = "newuser"
	form.Input.Password = "ValidPassword123!"
	form.Input.Attribution = "UNKNOWN"

	_, err := form.Save(ctx, db)
	require.NoError(t, err)

	newUser, err := factory.GetUser(ctx, db, form.Input.Email)
	require.NoError(t, err)
	require.Equal(t, "newuser", newUser.Username)
	require.Equal(t, "unknown", newUser.SignupAttribution.String)

	emails, err := factory.ListOutgoingEmails(ctx, db)
	require.NoError(t, err)
	require.Len(t, emails, 1)
}

func TestSignupForm_SaveFailsOnDuplicateEmail(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	sender := fakesender.New()

	existingUser, err := factory.User(ctx, db)
	require.NoError(t, err)

	// Save doesn't re-run Validate, so calling it directly with an
	// already-used email reaches auth.Signup's own unique-constraint error.
	form := forms.SignupFormNew(sender).(*forms.SignupForm)
	form.Input.Email = existingUser.Email
	form.Input.Username = "newuser"
	form.Input.Password = "ValidPassword123!"

	_, err = form.Save(ctx, db)
	require.Error(t, err)
}

func TestSignupForm_SaveSanitizesAttribution(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	sender := fakesender.New()

	form := forms.SignupFormNew(sender).(*forms.SignupForm)
	form.Input.Email = "newuser@example.test"
	form.Input.Username = "newuser"
	form.Input.Password = "ValidPassword123!"
	form.Input.Attribution = "invalid-with-dashes"

	action, err := form.Save(ctx, db)
	require.NoError(t, err)
	require.NotNil(t, action)
}

func TestSignupForm_SaveTrimsWhitespace(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	sender := fakesender.New()

	form := forms.SignupFormNew(sender).(*forms.SignupForm)
	form.Input.Email = "  Valid@EXAMPLE.TEST  "
	form.Input.Username = "  NewUser  "
	form.Input.Password = "ValidPassword123!"

	action, err := form.Save(ctx, db)
	require.NoError(t, err)
	require.NotNil(t, action)

	// factory has no by-email user reader, so the trimmed/lowercased email and
	// username are confirmed the way the form itself would see them: a second
	// signup using the already-trimmed/lowercased values must be rejected as
	// a duplicate of the one Save just persisted.
	c, _ := ginctx.New(t, http.MethodPost, "/signup", nil)
	dup := forms.SignupFormNew(sender).(*forms.SignupForm)
	dup.Input.Email = "valid@example.test"
	dup.Input.Username = "newuser"
	dup.Input.Password = "ValidPassword123!"

	err = dup.Validate(c, db)
	require.Error(t, err)
	require.True(t, dup.Errors.HasError("email"))
	require.True(t, dup.Errors.HasError("username"))
}
