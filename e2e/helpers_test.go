package e2e_test

import (
	"context"
	"testing"

	"github.com/can3p/pcom/e2e"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/stretchr/testify/require"
)

// testPassword is the password of every user made by newUser.
const testPassword = "secret-pw"

// newUser creates a user who can log in with testPassword.
func newUser(t *testing.T, app *e2e.App, opts ...factory.UserOpt) *core.User {
	t.Helper()

	u, err := factory.User(context.Background(), app.DB, append([]factory.UserOpt{factory.WithPassword(testPassword)}, opts...)...)
	require.NoError(t, err)

	return u
}

// loginAs returns a client logged in as u.
func loginAs(t *testing.T, app *e2e.App, u *core.User) *e2e.Client {
	t.Helper()

	c := app.Client(t)
	c.LoginAs(u.Email)

	return c
}

// newLoggedIn creates a user and a client logged in as them.
func newLoggedIn(t *testing.T, app *e2e.App) (*core.User, *e2e.Client) {
	t.Helper()

	u := newUser(t, app)

	return u, loginAs(t, app, u)
}
