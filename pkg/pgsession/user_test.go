package pgsession_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/can3p/pcom/pkg/pgsession"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// newContext returns a bare request context: pgsession needs no session or
// middleware, and ginctx would pull in the whole web stack.
func newContext() *gin.Context {
	gin.SetMode(gin.TestMode)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	return c
}

func TestGetUser_NoUserInContext(t *testing.T) {
	t.Parallel()

	c := newContext()

	require.Nil(t, pgsession.GetUser(c))
}

func TestSetUser_PopulatesContext(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	user := testutil.Must(factory.User(ctx, db))(t)

	c := newContext()

	require.NoError(t, pgsession.SetUser(c, db, user.ID))

	got := pgsession.GetUser(c)
	require.NotNil(t, got)
	require.Equal(t, user.ID, got.DBUser.ID)
	require.Equal(t, user.Email, got.DBUser.Email)
}

func TestSetUser_UnknownUserReturnsErrorAndLeavesContextEmpty(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB

	c := newContext()

	require.Error(t, pgsession.SetUser(c, db, "does-not-exist"))
	require.Nil(t, pgsession.GetUser(c), "a failed SetUser must not leave a stale context value")
}
