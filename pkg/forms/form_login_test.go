package forms_test

import (
	"context"
	gogoforms "github.com/can3p/gogo/forms"
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

const (
	testSalt     = "test-salt"
	testSiteRoot = "https://site.test"
)

// codeAccounts is the accounts service with the code key logging in needs.
func codeAccounts(db *sqlx.DB) *accounts.Service {
	return accounts.New(repo.New(db), fakesender.New(), nil, accounts.WithCodeKey("test-key"))
}

func TestLoginForm_ValidateNeedsAnEmail(t *testing.T) {
	t.Parallel()

	c, _ := ginctx.New(t, http.MethodPost, "/login", nil)
	form := forms.LoginFormNew(nil, testSalt).(*forms.LoginForm)
	form.Input.Email = "  "

	require.ErrorIs(t, form.Validate(c), gogoforms.ErrValidationFailed)
	require.True(t, form.Errors.HasError("email"))
}

// A known and an unknown address are answered the same way: an attempt is
// kept in the session and the code form follows.
func TestLoginForm_SaveStartsAnAttemptForAnyAddress(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	user := testutil.Must(factory.User(context.Background(), db))(t)

	for _, email := range []string{user.Email, "nobody@example.test"} {
		c, _ := ginctx.New(t, http.MethodPost, "/login", nil)
		form := forms.LoginFormNew(codeAccounts(db), testSalt).(*forms.LoginForm)
		form.Input.Email = email

		action, err := form.Save(c)
		require.NoError(t, err)
		require.NotNil(t, action)
		require.NotEmpty(t, auth.LoginAttempt(c), email)
	}
}
