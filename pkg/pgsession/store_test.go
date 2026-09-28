package pgsession_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/can3p/pcom/pkg/pgsession"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	gsessions "github.com/gin-contrib/sessions"
	"github.com/stretchr/testify/require"
)

// TestNewStore_RoundTrip drives the Store interface the way gin-contrib's
// session middleware does: New a session, set a value, Save it (which sets
// a cookie), then Get it back with that cookie on a fresh request. This
// pins the pgstore-backed round trip through Postgres, not just the
// in-memory gorilla/sessions plumbing.
func TestNewStore_RoundTrip(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	store := pgsession.NewStore(db, []byte("0123456789abcdef0123456789abcdef"))

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

	db := testdb.New(t).DB
	store := pgsession.NewStore(db, []byte("0123456789abcdef0123456789abcdef"))

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

	db := testdb.New(t).DB
	store := pgsession.NewStore(db, []byte("0123456789abcdef0123456789abcdef"))

	store.Options(gsessions.Options{Path: "/custom", MaxAge: 3600})

	req := httptest.NewRequest(http.MethodGet, "/", nil)

	sess, err := store.New(req, "sess")
	require.NoError(t, err)
	require.Equal(t, "/custom", sess.Options.Path)
	require.Equal(t, 3600, sess.Options.MaxAge)
}
