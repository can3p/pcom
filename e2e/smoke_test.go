package e2e_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/can3p/pcom/e2e"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/stretchr/testify/require"
)

func TestSmoke_AnonymousHome(t *testing.T) {
	app := e2e.Start(t)

	resp := app.Client(t).Get("/").RequireStatus(http.StatusOK)

	require.NotZero(t, resp.Doc().Find("body").Length())
}

func TestSmoke_LoginReachesFeed(t *testing.T) {
	app := e2e.Start(t)
	user, err := factory.User(context.Background(), app.DB, factory.WithPassword("secret-pw"))
	require.NoError(t, err)

	client := app.Client(t)
	client.Get("/feed").RequireStatus(http.StatusFound)

	client.LoginAs(user.Email, "secret-pw")

	client.Get("/feed").RequireStatus(http.StatusOK)
}
