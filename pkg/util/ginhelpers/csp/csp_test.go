package csp_test

import (
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/can3p/pcom/pkg/testutil/ginctx"
	"github.com/can3p/pcom/pkg/util/ginhelpers/csp"
	"github.com/stretchr/testify/require"
)

// withoutFlyAppName makes sure FLY_APP_NAME is unset for the duration of the
// test, restoring whatever value (or absence) it had before.
func withoutFlyAppName(t *testing.T) {
	t.Helper()

	orig, had := os.LookupEnv("FLY_APP_NAME")
	require.NoError(t, os.Unsetenv("FLY_APP_NAME"))

	t.Cleanup(func() {
		if had {
			_ = os.Setenv("FLY_APP_NAME", orig)
		}
	})
}

func TestGetNonces_NilBeforeCsp(t *testing.T) {
	t.Parallel()

	c, _ := ginctx.New(t, http.MethodGet, "/", nil)

	require.Nil(t, csp.GetStyleNonce(c))
	require.Nil(t, csp.GetScriptNonce(c))
}

func TestCsp_SetsHeaderShapeAndContextNonces(t *testing.T) {
	withoutFlyAppName(t)

	c, w := ginctx.New(t, http.MethodGet, "/", nil)

	csp.Csp(c)

	header := w.Header().Get("Content-Security-Policy")
	require.NotEmpty(t, header)
	require.Contains(t, header, "default-src 'self'")
	require.Contains(t, header, "frame-ancestors 'none';")
	require.Contains(t, header, "script-src 'self'")
	require.Contains(t, header, "style-src 'self'")

	require.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
	require.Empty(t, w.Header().Get("Strict-Transport-Security"))

	styleNonce := csp.GetStyleNonce(c)
	scriptNonce := csp.GetScriptNonce(c)
	require.NotNil(t, styleNonce)
	require.NotNil(t, scriptNonce)
	require.NotEqual(t, *styleNonce, *scriptNonce)

	require.Contains(t, header, "'nonce-"+*styleNonce+"'")
	require.Contains(t, header, "'nonce-"+*scriptNonce+"'")
}

func TestCsp_InCluster_SetsHSTS(t *testing.T) {
	t.Setenv("FLY_APP_NAME", "pcom-test")

	c, w := ginctx.New(t, http.MethodGet, "/", nil)

	csp.Csp(c)

	require.Equal(t, "max-age=31536000; includeSubDomains", w.Header().Get("Strict-Transport-Security"))
}

func TestCsp_FreshNoncesPerRequest(t *testing.T) {
	withoutFlyAppName(t)

	c1, w1 := ginctx.New(t, http.MethodGet, "/", nil)
	csp.Csp(c1)

	c2, w2 := ginctx.New(t, http.MethodGet, "/", nil)
	csp.Csp(c2)

	style1, script1 := csp.GetStyleNonce(c1), csp.GetScriptNonce(c1)
	style2, script2 := csp.GetStyleNonce(c2), csp.GetScriptNonce(c2)

	require.NotEqual(t, *style1, *style2)
	require.NotEqual(t, *script1, *script2)

	// Sanity: the headers actually carry these distinct nonces too.
	require.False(t, strings.Contains(w1.Header().Get("Content-Security-Policy"), *style2))
	require.False(t, strings.Contains(w2.Header().Get("Content-Security-Policy"), *style1))
}
