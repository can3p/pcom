package csp_test

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/can3p/pcom/pkg/testutil/ginctx"
	"github.com/can3p/pcom/pkg/util/ginhelpers/csp"
	"github.com/stretchr/testify/require"
)

func TestGetNonces_NilBeforeCsp(t *testing.T) {
	t.Parallel()

	c, _ := ginctx.New(t, http.MethodGet, "/", nil)

	require.Nil(t, csp.GetStyleNonce(c))
	require.Nil(t, csp.GetScriptNonce(c))
}

func TestCsp_SetsHeaderShapeAndContextNonces(t *testing.T) {
	c, w := ginctx.New(t, http.MethodGet, "/", nil)

	csp.New(csp.Options{})(c)

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

func TestCsp_Options(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		opts csp.Options
		hsts bool
		cdn  []string
	}{
		{name: "HSTS on", opts: csp.Options{HSTS: true}, hsts: true},
		{name: "HSTS off", opts: csp.Options{}},
		{name: "CDNs set", opts: csp.Options{StaticCDN: "https://static.example", MediaCDN: "https://media.example"}, cdn: []string{"https://static.example", "https://media.example"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			c, w := ginctx.New(t, http.MethodGet, "/", nil)
			csp.New(tc.opts)(c)

			require.Equal(t, tc.hsts, w.Header().Get("Strict-Transport-Security") == "max-age=31536000; includeSubDomains")

			header := w.Header().Get("Content-Security-Policy")
			for _, cdn := range []string{"https://static.example", "https://media.example"} {
				require.Equal(t, slices.Contains(tc.cdn, cdn), strings.Contains(header, cdn), cdn)
			}
		})
	}
}

func TestCsp_FreshNoncesPerRequest(t *testing.T) {
	c1, w1 := ginctx.New(t, http.MethodGet, "/", nil)
	csp.New(csp.Options{})(c1)

	c2, w2 := ginctx.New(t, http.MethodGet, "/", nil)
	csp.New(csp.Options{})(c2)

	style1, script1 := csp.GetStyleNonce(c1), csp.GetScriptNonce(c1)
	style2, script2 := csp.GetStyleNonce(c2), csp.GetScriptNonce(c2)

	require.NotEqual(t, *style1, *style2)
	require.NotEqual(t, *script1, *script2)

	// Sanity: the headers actually carry these distinct nonces too.
	require.False(t, strings.Contains(w1.Header().Get("Content-Security-Policy"), *style2))
	require.False(t, strings.Contains(w2.Header().Get("Content-Security-Policy"), *style1))
}
