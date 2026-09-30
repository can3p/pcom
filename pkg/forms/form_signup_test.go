package forms_test

import (
	"context"
	"net/http"
	"testing"

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
		setup        func(t *testing.T) (email, username, password string)
		wantErrField string
	}{
		{"empty email", func(t *testing.T) (string, string, string) {
			return "", "newuser", "ValidPassword123!"
		}, "email"},
		{"existing email", func(t *testing.T) (string, string, string) {
			u := testutil.Must(factory.User(ctx, db))(t)
			return u.Email, "newuser", "ValidPassword123!"
		}, "email"},
		{"empty username", func(t *testing.T) (string, string, string) {
			return "valid@example.test", "", "ValidPassword123!"
		}, "username"},
		{"existing username", func(t *testing.T) (string, string, string) {
			u := testutil.Must(factory.User(ctx, db))(t)
			return "valid@example.test", u.Username, "ValidPassword123!"
		}, "username"},
		{"invalid username format", func(t *testing.T) (string, string, string) {
			return "valid@example.test", "1abc", "ValidPassword123!"
		}, "username"},
		{"weak password", func(t *testing.T) (string, string, string) {
			return "valid@example.test", "newuser", "short"
		}, "password"},
		{"empty password", func(t *testing.T) (string, string, string) {
			return "valid@example.test", "newuser", ""
		}, "password"},
		{"success", func(t *testing.T) (string, string, string) {
			return "valid@example.test", "newuser", "ValidPassword123!"
		}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c, _ := ginctx.New(t, http.MethodPost, "/signup", nil)
			email, username, password := tt.setup(t)

			form := forms.SignupFormNew(accountsFor(db, fakesender.New())).(*forms.SignupForm)
			form.Input.Email = email
			form.Input.Username = username
			form.Input.Password = password

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
// anchored regex validation.AttributionRE is meant to reject outright
// ("invalid-with-dashes" contains characters outside [a-z_]). Today's
// unanchored regex matches a substring of it, so Save keeps the raw value
// instead of falling back to "unknown"; this is the only assertion below
// that fails until #148 is fixed. The mail assertions are not affected by
// the bug and are expected to pass already.
func TestSignupForm_SaveSanitizesInvalidAttribution(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	sender := fakesender.New()

	form := forms.SignupFormNew(accountsFor(db, sender)).(*forms.SignupForm)
	form.Input.Email = "newuser@example.test"
	form.Input.Username = "newuser"
	form.Input.Password = "ValidPassword123!"
	form.Input.Attribution = "invalid-with-dashes"

	action, err := form.Save(ctx)
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
}

func TestSignupForm_SaveTrimsWhitespace(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	sender := fakesender.New()

	form := forms.SignupFormNew(accountsFor(db, sender)).(*forms.SignupForm)
	form.Input.Email = "  Valid@EXAMPLE.TEST  "
	form.Input.Username = "  NewUser  "
	form.Input.Password = "ValidPassword123!"

	action, err := form.Save(ctx)
	require.NoError(t, err)
	require.NotNil(t, action)

	// factory has no by-email user reader, so the trimmed/lowercased email and
	// username are confirmed the way the form itself would see them: a second
	// signup using the already-trimmed/lowercased values must be rejected as
	// a duplicate of the one Save just persisted.
	c, _ := ginctx.New(t, http.MethodPost, "/signup", nil)
	dup := forms.SignupFormNew(accountsFor(db, sender)).(*forms.SignupForm)
	dup.Input.Email = "valid@example.test"
	dup.Input.Username = "newuser"
	dup.Input.Password = "ValidPassword123!"

	err = dup.Validate(c)
	require.Error(t, err)
	require.True(t, dup.Errors.HasError("email"))
	require.True(t, dup.Errors.HasError("username"))
}
