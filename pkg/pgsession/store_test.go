package pgsession_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/can3p/pcom/pkg/pgsession"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	gsessions "github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newStore(t *testing.T) pgsession.Store {
	t.Helper()
	return pgsession.NewStore(testdb.New(t).DB, []byte("0123456789abcdef0123456789abcdef"))
}

// TestNewStore_RoundTrip drives the Store interface the way gin-contrib's
// session middleware does: New a session, set a value, Save it (which sets
// a cookie), then Get it back with that cookie on a fresh request. This
// pins the pgstore-backed round trip through Postgres, not just the
// in-memory gorilla/sessions plumbing.
func TestNewStore_RoundTrip(t *testing.T) {
	t.Parallel()

	store := newStore(t)

	req := httptest.NewRequest(http.MethodGet, "/", nil)

	sess, err := store.New(req, "sess")
	require.NoError(t, err)
	require.True(t, sess.IsNew, "a session with no cookie on the request is new")

	sess.Values["user"] = "user-id-1"

	w := httptest.NewRecorder()
	require.NoError(t, store.Save(req, w, sess))

	cookies := w.Result().Cookies()
	require.Len(t, cookies, 1, "Save must set the session cookie")

	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, c := range cookies {
		req2.AddCookie(c)
	}

	got, err := store.Get(req2, "sess")
	require.NoError(t, err)
	require.False(t, got.IsNew, "a request carrying the cookie must load the saved session")
	require.Equal(t, "user-id-1", got.Values["user"])
}

// TestNewStore_GetWithoutCookieReturnsFreshSession pins that a request with
// no session cookie at all behaves like New, rather than erroring.
func TestNewStore_GetWithoutCookieReturnsFreshSession(t *testing.T) {
	t.Parallel()

	store := newStore(t)

	req := httptest.NewRequest(http.MethodGet, "/", nil)

	sess, err := store.Get(req, "sess")
	require.NoError(t, err)
	require.True(t, sess.IsNew)
	require.Empty(t, sess.Values)
}

// TestNewStore_OptionsAppliedToNewSessions pins that Options bridges the
// gin-contrib Options into gorilla's, and that a session minted afterwards
// carries them.
func TestNewStore_OptionsAppliedToNewSessions(t *testing.T) {
	t.Parallel()

	store := newStore(t)
	store.Options(gsessions.Options{Path: "/custom", MaxAge: 3600})

	req := httptest.NewRequest(http.MethodGet, "/", nil)

	sess, err := store.New(req, "sess")
	require.NoError(t, err)
	require.Equal(t, "/custom", sess.Options.Path)
	require.Equal(t, 3600, sess.Options.MaxAge)
}

// TestRegenerate pins #122: rotating a session at login issues a new
// session ID and deletes the old one, so a session ID planted before login
// is worth nothing afterwards.
func TestRegenerate(t *testing.T) {
	t.Parallel()

	store := newStore(t)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(gsessions.Sessions("sess", store))
	r.GET("/visit", func(c *gin.Context) {
		s := gsessions.Default(c)
		s.Set("csrf_token", "before")
		require.NoError(t, s.Save())
	})
	r.GET("/login", func(c *gin.Context) {
		s := gsessions.Default(c)
		require.NoError(t, pgsession.Regenerate(s))
		require.Nil(t, s.Get("csrf_token"), "the old values must not carry over")
		s.Set("user", "user-id-1")
		require.NoError(t, s.Save())
	})

	serve := func(path string, cookies []*http.Cookie) []*http.Cookie {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		for _, c := range cookies {
			req.AddCookie(c)
		}

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)

		return w.Result().Cookies()
	}

	before := serve("/visit", nil)
	require.Len(t, before, 1)

	after := serve("/login", before)
	require.Len(t, after, 1)
	require.NotEqual(t, before[0].Value, after[0].Value, "login must issue a new session cookie")

	load := func(cookies []*http.Cookie) map[any]any {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		for _, c := range cookies {
			req.AddCookie(c)
		}

		sess, err := store.New(req, "sess")
		require.NoError(t, err)

		return sess.Values
	}

	require.Empty(t, load(before), "the old session must be deleted")
	require.Equal(t, "user-id-1", load(after)["user"])
}
