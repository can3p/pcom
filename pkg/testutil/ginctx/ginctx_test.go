package ginctx_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/pgsession"
	"github.com/can3p/pcom/pkg/testutil/ginctx"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/can3p/pcom/pkg/util/ginhelpers/csp"
	"github.com/gin-contrib/sessions"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/volatiletech/sqlboiler/v4/boil"
)

func TestNew_SessionWorks(t *testing.T) {
	t.Parallel()

	c, w := ginctx.New(t, http.MethodGet, "/", nil)
	require.NotNil(t, w)

	sess := sessions.Default(c)
	sess.Set("greeting", "hello")
	require.Equal(t, "hello", sess.Get("greeting"))
}

func TestNew_NoUserByDefault(t *testing.T) {
	t.Parallel()

	c, _ := ginctx.New(t, http.MethodGet, "/", nil)
	require.Nil(t, pgsession.GetUser(c))
}

func TestNew_WithUser(t *testing.T) {
	t.Parallel()

	testDB := testdb.New(t)
	ctx := context.Background()

	// Inserted directly with the generated model: pkg/testutil/factory is
	// being written in parallel and this package must not depend on it.
	user := &core.User{
		ID:       uuid.NewString(),
		Email:    "ginctx-user@example.com",
		Username: "ginctx-user",
		Timezone: "UTC",
	}
	require.NoError(t, user.Insert(ctx, testDB.DB, boil.Infer()))

	c, _ := ginctx.New(t, http.MethodGet, "/", nil, ginctx.WithUser(t, testDB.DB, user.ID))

	got := pgsession.GetUser(c)
	require.NotNil(t, got)
	require.Equal(t, user.ID, got.DBUser.ID)
}

func TestNew_WithCSPNonces(t *testing.T) {
	t.Parallel()

	c, _ := ginctx.New(t, http.MethodGet, "/", nil, ginctx.WithCSPNonces("style-nonce", "script-nonce"))

	stylePtr := csp.GetStyleNonce(c)
	scriptPtr := csp.GetScriptNonce(c)

	require.NotNil(t, stylePtr)
	require.NotNil(t, scriptPtr)
	require.Equal(t, "style-nonce", *stylePtr)
	require.Equal(t, "script-nonce", *scriptPtr)
}
