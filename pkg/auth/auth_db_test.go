package auth_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service/accounts"
	"github.com/jmoiron/sqlx"

	"github.com/can3p/pcom/pkg/auth"
	"github.com/can3p/pcom/pkg/pgsession"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/ginctx"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/gin-contrib/sessions"
	"github.com/stretchr/testify/require"
)

// svc is the accounts service over the test database; auth talks to users through it.
func svc(db *sqlx.DB) *accounts.Service {
	return accounts.New(repo.New(db), nil, nil)
}

func TestStartSession_PutsTheUserInTheSession(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	user := testutil.Must(factory.User(context.Background(), db))(t)

	c, _ := ginctx.New(t, http.MethodPost, "/login", nil)
	require.NoError(t, auth.StartSession(c, user))
	require.Equal(t, user.ID, sessions.Default(c).Get("user"))
}

func TestAuth_StaleSessionUserLogsAndContinues(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB

	c, _ := ginctx.New(t, http.MethodGet, "/", nil)
	sess := sessions.Default(c)
	sess.Set("user", "00000000-0000-0000-0000-000000000000")
	require.NoError(t, sess.Save())

	// A session pointing at a user that no longer exists must not abort the
	// request: Auth only logs the pgsession.SetUser failure and lets the
	// request continue as if anonymous.
	auth.Auth(c, svc(db))

	require.False(t, c.IsAborted())
	require.Nil(t, pgsession.GetUser(c))
}

func TestEnforceAuth_LoggedInUserPassesThrough(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	user := testutil.Must(factory.User(ctx, db))(t)

	c, _ := ginctx.New(t, http.MethodGet, "/controls", nil, ginctx.WithUser(t, db, user.ID))

	auth.EnforceAuth(c)

	require.False(t, c.IsAborted())
}

func TestGetUserData_LoggedInUser(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	user := testutil.Must(factory.User(ctx, db))(t)

	c, _ := ginctx.New(t, http.MethodGet, "/", nil, ginctx.WithUser(t, db, user.ID))

	data := auth.GetUserData(c)

	require.True(t, data.IsLoggedIn)
	require.NotEmpty(t, data.CSRFToken, "GetUserData mints a CSRF token for the session")

	// A second call on the same request/session must reuse the token
	// minted on the first call rather than minting a new one every time.
	again := auth.GetUserData(c)
	require.Equal(t, data.CSRFToken, again.CSRFToken)
}

func TestAuth_LoggedInSessionSetsPgsessionUser(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	user := testutil.Must(factory.User(ctx, db))(t)

	c, _ := ginctx.New(t, http.MethodGet, "/", nil)
	sess := sessions.Default(c)
	sess.Set("user", user.ID)
	require.NoError(t, sess.Save())

	auth.Auth(c, svc(db))

	require.False(t, c.IsAborted())

	got := pgsession.GetUser(c)
	require.NotNil(t, got)
	require.Equal(t, user.ID, got.DBUser.ID)
}

func TestAuthAPI_UnknownKeyReturnsForbidden(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB

	c, w := ginctx.New(t, http.MethodGet, "/api/whatever", nil)
	// A syntactically valid but unassigned API key (the column is a uuid),
	// so the lookup cleanly misses rather than failing to parse.
	c.Request.Header.Set("Authorization", "Bearer 00000000-0000-0000-0000-000000000000")

	auth.AuthAPI(c, svc(db))

	require.True(t, c.IsAborted())
	require.Equal(t, http.StatusForbidden, w.Code)
	require.Nil(t, pgsession.GetUser(c))
}

func TestAuthAPI_DBErrorReturnsInternalServerError(t *testing.T) {
	t.Parallel()

	testDB := testdb.New(t)
	db := testDB.DB
	require.NoError(t, db.Close())

	c, w := ginctx.New(t, http.MethodGet, "/api/whatever", nil)
	c.Request.Header.Set("Authorization", "Bearer 00000000-0000-0000-0000-000000000000")

	auth.AuthAPI(c, svc(db))

	require.True(t, c.IsAborted())
	require.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestAuthAPI_KnownKeySetsUser(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	user := testutil.Must(factory.User(ctx, db))(t)

	key := testutil.Must(factory.APIKey(ctx, db, user.ID))(t)

	c, w := ginctx.New(t, http.MethodGet, "/api/whatever", nil)
	c.Request.Header.Set("Authorization", "Bearer "+key.APIKey)

	auth.AuthAPI(c, svc(db))

	require.False(t, c.IsAborted())
	require.NotEqual(t, http.StatusForbidden, w.Code)

	got := pgsession.GetUser(c)
	require.NotNil(t, got)
	require.Equal(t, user.ID, got.DBUser.ID)
}
