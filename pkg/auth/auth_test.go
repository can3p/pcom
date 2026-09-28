package auth_test

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/can3p/pcom/pkg/auth"
	"github.com/can3p/pcom/pkg/testutil/ginctx"
	"github.com/gin-contrib/sessions"
	"github.com/stretchr/testify/require"
)

func TestHashValue(t *testing.T) {
	t.Setenv("SESSION_SALT", "salt-one")

	h1 := auth.HashValue("/some/path")
	h2 := auth.HashValue("/some/path")
	require.Equal(t, h1, h2, "hashing the same value twice must be deterministic")
	require.Len(t, h1, 64, "sha256 hex digest is 64 chars")

	require.NotEqual(t, h1, auth.HashValue("/some/other/path"), "different inputs must hash differently")

	t.Setenv("SESSION_SALT", "salt-two")
	require.NotEqual(t, h1, auth.HashValue("/some/path"), "different salt must change the hash")
}

func TestRedirectToLogin(t *testing.T) {
	t.Setenv("SESSION_SALT", "test-salt")

	c, w := ginctx.New(t, http.MethodGet, "/feed", nil)

	auth.RedirectToLogin(c)

	require.Equal(t, http.StatusFound, w.Code)

	loc, err := url.Parse(w.Header().Get("Location"))
	require.NoError(t, err)
	require.Equal(t, "/login", loc.Path)

	q := loc.Query()
	require.Equal(t, "/feed", q.Get("return_url"))
	require.Equal(t, auth.HashValue("/feed"), q.Get("sign"))
}

func TestEnforceReferer(t *testing.T) {
	t.Setenv("SITE_ROOT", "https://example.com")

	tests := []struct {
		name       string
		referer    string
		wantAbort  bool
		wantStatus int
	}{
		{
			name:       "no referer",
			referer:    "",
			wantAbort:  true,
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "referer from a different site",
			referer:    "https://evil.example/feed",
			wantAbort:  true,
			wantStatus: http.StatusNotFound,
		},
		{
			name:      "referer from the site itself",
			referer:   "https://example.com/feed",
			wantAbort: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, w := ginctx.New(t, http.MethodGet, "/controls/action/whatever", nil)
			if tt.referer != "" {
				c.Request.Header.Set("referer", tt.referer)
			}

			auth.EnforceReferer(c)

			require.Equal(t, tt.wantAbort, c.IsAborted())
			if tt.wantAbort {
				require.Equal(t, tt.wantStatus, w.Code)
			}
		})
	}
}

func TestEnforceAuth_RedirectsAnonymousUser(t *testing.T) {
	t.Setenv("SESSION_SALT", "test-salt")

	c, w := ginctx.New(t, http.MethodGet, "/controls", nil)

	auth.EnforceAuth(c)

	require.True(t, c.IsAborted())
	require.Equal(t, http.StatusFound, w.Code)

	loc, err := url.Parse(w.Header().Get("Location"))
	require.NoError(t, err)
	require.Equal(t, "/login", loc.Path)
	require.Equal(t, "/controls", loc.Query().Get("return_url"))
}

func TestAuthAPI_BadRequestPaths(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		header string
	}{
		{name: "no Authorization header", header: ""},
		{name: "missing token", header: "Bearer"},
		{name: "wrong scheme", header: "Basic sometoken"},
		{name: "too many parts", header: "Bearer token extra"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c, w := ginctx.New(t, http.MethodGet, "/api/whatever", nil)
			if tt.header != "" {
				c.Request.Header.Set("Authorization", tt.header)
			}

			// These inputs are all rejected before the handler ever
			// touches the database, so a nil *sqlx.DB is safe here.
			auth.AuthAPI(c, nil)

			require.True(t, c.IsAborted())
			require.Equal(t, http.StatusBadRequest, w.Code)
		})
	}
}

func TestLogout_LoggedInUser(t *testing.T) {
	t.Parallel()

	c, w := ginctx.New(t, http.MethodGet, "/logout", nil)

	sess := sessions.Default(c)
	sess.Set("user", "some-user-id")
	require.NoError(t, sess.Save())

	auth.Logout(c)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "/", w.Header().Get("HX-Redirect"))
	require.True(t, c.IsAborted())

	require.Nil(t, sessions.Default(c).Get("user"), "logout must clear the session user key")
}

func TestLogout_AnonymousUser(t *testing.T) {
	t.Parallel()

	c, w := ginctx.New(t, http.MethodGet, "/logout", nil)

	auth.Logout(c)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "/", w.Header().Get("HX-Redirect"))
	require.False(t, c.IsAborted(), "logout with no session user has nothing to abort for")
}

func TestAddFlashAndGetFlashes(t *testing.T) {
	t.Parallel()

	c, _ := ginctx.New(t, http.MethodGet, "/", nil)

	require.Empty(t, auth.GetFlashes(c), "no flashes before any is added")

	auth.AddFlash(c, "hello there")

	flashes := auth.GetFlashes(c)
	require.Equal(t, []any{"hello there"}, flashes)

	require.Empty(t, auth.GetFlashes(c), "flashes are consumed once read")
}

func TestGetUserData_AnonymousUser(t *testing.T) {
	t.Parallel()

	c, _ := ginctx.New(t, http.MethodGet, "/", nil)

	data := auth.GetUserData(c)

	require.False(t, data.IsLoggedIn)
	require.Nil(t, data.User)
	require.Nil(t, data.DBUser)
	require.NotEmpty(t, data.CSRFToken)

	// A second call on the same request must reuse the token that was
	// minted for the session on the first call rather than minting a new
	// one every time.
	again := auth.GetUserData(c)
	require.Equal(t, data.CSRFToken, again.CSRFToken)
}

func TestGetAPIUserData_AnonymousUser(t *testing.T) {
	t.Parallel()

	c, _ := ginctx.New(t, http.MethodGet, "/", nil)

	data := auth.GetAPIUserData(c)

	require.False(t, data.IsLoggedIn)
	require.Nil(t, data.User)
	require.Nil(t, data.DBUser)
}

func TestAuth_NoSessionUserSkipsDB(t *testing.T) {
	t.Parallel()

	c, _ := ginctx.New(t, http.MethodGet, "/", nil)

	// No session user is set, so Auth must return before it ever
	// dereferences db, which is why passing nil here is safe.
	auth.Auth(c, nil)

	require.False(t, c.IsAborted())
}
