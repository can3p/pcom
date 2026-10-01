package forms_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/can3p/pcom/pkg/auth"
	"github.com/can3p/pcom/pkg/forms"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service/accounts"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/fakesender"
	"github.com/can3p/pcom/pkg/testutil/ginctx"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
)

// accountsFor is the accounts service the forms under test call, over db.
func accountsFor(db *sqlx.DB, s repo.MailQueue) *accounts.Service {
	return accounts.New(repo.New(db), s, nil)
}

func TestSignupForm_Validate(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	tests := []struct {
		name         string
		setup        func(t *testing.T) (email, username string)
		wantErrField string
	}{
		{"empty email", func(t *testing.T) (string, string) {
			return "", "newuser"
		}, "email"},
		{"existing email", func(t *testing.T) (string, string) {
			u := testutil.Must(factory.User(ctx, db))(t)
			return u.Email, "newuser"
		}, "email"},
		{"empty username", func(t *testing.T) (string, string) {
			return "valid@example.test", ""
		}, "username"},
		{"existing username", func(t *testing.T) (string, string) {
			u := testutil.Must(factory.User(ctx, db))(t)
			return "valid@example.test", u.Username
		}, "username"},
		{"invalid username format", func(t *testing.T) (string, string) {
			return "valid@example.test", "1abc"
		}, "username"},
		{"success", func(t *testing.T) (string, string) {
			return "valid@example.test", "newuser"
		}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c, _ := ginctx.New(t, http.MethodPost, "/signup", nil)
			email, username := tt.setup(t)

			form := forms.SignupFormNew(codeAccounts(db, fakesender.New())).(*forms.SignupForm)
			form.Input.Email = email
			form.Input.Username = username

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

// TestSignupForm_SaveSanitizesInvalidAttribution uses an attribution the
// anchored regex validation.AttributionRE rejects ("invalid-with-dashes"
// contains characters outside [a-z_]): Save falls back to "unknown".
func TestSignupForm_SaveSanitizesInvalidAttribution(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	sender := fakesender.New()

	form := forms.SignupFormNew(codeAccounts(db, sender)).(*forms.SignupForm)
	form.Input.Email = "newuser@example.test"
	form.Input.Username = "newuser"
	form.Input.Attribution = "invalid-with-dashes"

	c, _ := ginctx.New(t, http.MethodPost, "/signup", nil)

	action, err := form.Save(c)
	require.NoError(t, err)
	require.NotNil(t, action)

	newUser := testutil.Must(factory.GetUserByEmail(ctx, db, "newuser@example.test"))(t)
	require.Equal(t, "newuser", newUser.Username)
	require.Equal(t, "unknown", newUser.SignupAttribution.String)

	sent := sender.Sent()
	require.Len(t, sent, 2)
	require.Equal(t, "admin_new_user", sent[0].EmailType)
	require.Equal(t, "confirm_signup", sent[1].EmailType)
	require.Equal(t, newUser.Email, sent[1].Mail.To[0].Address)
	require.NotEmpty(t, auth.LoginAttempt(c), "the visitor's session carries the attempt the code form finishes")
}

func TestSignupForm_SaveTrimsWhitespace(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	sender := fakesender.New()

	c, _ := ginctx.New(t, http.MethodPost, "/signup", nil)

	form := forms.SignupFormNew(codeAccounts(db, sender)).(*forms.SignupForm)
	form.Input.Email = "  Valid@EXAMPLE.TEST  "
	form.Input.Username = "  NewUser  "

	action, err := form.Save(c)
	require.NoError(t, err)
	require.NotNil(t, action)

	// factory has no by-email user reader, so the trimmed/lowercased email and
	// username are confirmed the way the form itself would see them: a second
	// signup using the already-trimmed/lowercased values must be rejected as
	// a duplicate of the one Save just persisted.
	dup := forms.SignupFormNew(codeAccounts(db, sender)).(*forms.SignupForm)
	dup.Input.Email = "valid@example.test"
	dup.Input.Username = "newuser"

	err = dup.Validate(c)
	require.Error(t, err)
	require.True(t, dup.Errors.HasError("email"))
	require.True(t, dup.Errors.HasError("username"))
}
