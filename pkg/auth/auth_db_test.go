package auth_test

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service/accounts"
	"github.com/jmoiron/sqlx"

	"github.com/can3p/pcom/pkg/auth"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/pgsession"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/ginctx"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/gin-contrib/sessions"
	"github.com/stretchr/testify/require"
	"github.com/volatiletech/null/v8"
)

// svc is the accounts service over the test database; auth talks to users through it.
func svc(db *sqlx.DB) *accounts.Service {
	return accounts.New(repo.New(db), nil, nil)
}

func TestLogin(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	user := testutil.Must(factory.User(ctx, db, factory.WithPassword("s3cr3t-pw")))(t)

	cases := []struct {
		name     string
		password string
		wantErr  bool
	}{
		{name: "sets session on success", password: "s3cr3t-pw"},
		{name: "bad credentials returns error without setting session", password: "wrong-pw", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			c, _ := ginctx.New(t, http.MethodPost, "/login", nil)

			err := auth.Login(c, svc(db), user.Email, tc.password)
			if tc.wantErr {
				require.Error(t, err)
				require.Nil(t, sessions.Default(c).Get("user"))
			} else {
				require.NoError(t, err)
				require.Equal(t, user.ID, sessions.Default(c).Get("user"))
			}
		})
	}
}

// TestLogin_EmailCaseAndLegacyHashes pins #114 and #119. The email is
// matched whatever case and surrounding space the user types. A legacy
// sha256 hash, computed from the stored email, is replaced by argon2id at
// login.
func TestLogin_EmailCaseAndLegacyHashes(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	for i, typed := range []string{"%s@x.test", "%s@X.TEST", " %s@x.test "} {
		t.Run(typed, func(t *testing.T) {
			t.Parallel()

			local := fmt.Sprintf("user%d", i)
			email := local + "@x.test"
			id := testutil.Must(factory.User(ctx, db, factory.WithEmail(email), factory.WithPassword("pw")))(t).ID
			typed := fmt.Sprintf(typed, strings.ToUpper(local[:1])+local[1:])

			c, _ := ginctx.New(t, http.MethodPost, "/login", nil)
			require.NoError(t, auth.Login(c, svc(db), typed, "pw"))
			require.Equal(t, id, sessions.Default(c).Get("user"))

			got := testutil.Must(factory.GetUser(ctx, db, id))(t)
			require.True(t, strings.HasPrefix(got.Pwdhash.String, "$argon2id$"), "a legacy hash is replaced at login")
			require.Equal(t, email, got.Email)

			c2, _ := ginctx.New(t, http.MethodPost, "/login", nil)
			require.NoError(t, auth.Login(c2, svc(db), typed, "pw"), "the new hash logs in too")
			require.Equal(t, id, sessions.Default(c2).Get("user"))
			require.Error(t, auth.Login(c2, svc(db), typed, "wrong-pw"))
		})
	}
}

func TestLogin_DBErrorIsReturnedAsIs(t *testing.T) {
	t.Parallel()

	testDB := testdb.New(t)
	db := testDB.DB
	require.NoError(t, db.Close())

	c, _ := ginctx.New(t, http.MethodPost, "/login", nil)

	var err error
	require.NotPanics(t, func() {
		err = auth.Login(c, svc(db), "someone@example.test", "pw")
	})
	require.Error(t, err, "a DB error should be returned to the caller, not mistaken for bad credentials")
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

func TestLogin_AccountWithoutPasswordCannotLogIn(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	user := testutil.Must(factory.User(ctx, db, func(u *core.User) { u.Pwdhash = null.String{} }))(t)

	c, _ := ginctx.New(t, http.MethodPost, "/login", nil)
	require.Error(t, auth.Login(c, svc(db), user.Email, ""))
}
