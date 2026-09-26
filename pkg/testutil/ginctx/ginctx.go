// Package ginctx builds a *gin.Context for a handler test, wired up the way
// cmd/web/main.go wires a real request: a cookie-backed session under the
// name "sess". Options add what else a handler under test expects, such as
// a logged-in user or CSP nonces.
package ginctx

import (
	"io"
	"net/http/httptest"
	"testing"

	"github.com/can3p/pcom/pkg/pgsession"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
)

// sessionName is the name gin-contrib/sessions installs the store under,
// matching cmd/web/main.go's sessions.Sessions("sess", store).
const sessionName = "sess"

// CSP nonce context keys, matching pkg/util/ginhelpers/csp, which does not
// export them.
const (
	styleNonceKey  = "csp_style_nonce"
	scriptNonceKey = "csp_script_nonce"
)

// Option configures the *gin.Context New builds, after its request and
// session are installed.
type Option func(*gin.Context)

// WithUser makes pgsession.GetUser(c) return userID's user for the rest of
// the request, the way auth.Auth does once it reads a logged-in session.
// db must already hold a row for userID: pgsession.SetUser looks it up
// eagerly and fails the test if that lookup errors.
func WithUser(t testing.TB, db *sqlx.DB, userID string) Option {
	return func(c *gin.Context) {
		t.Helper()
		require.NoError(t, pgsession.SetUser(c, db, userID))
	}
}

// WithCSPNonces sets the request's CSP style/script nonces to fixed values,
// the way pkg/util/ginhelpers/csp.Csp does with random ones, so a test can
// assert on a template that renders them.
func WithCSPNonces(style, script string) Option {
	return func(c *gin.Context) {
		c.Set(styleNonceKey, style)
		c.Set(scriptNonceKey, script)
	}
}

// New returns a *gin.Context for method/target with body as the request
// body, and the *httptest.ResponseRecorder backing its writer. The context
// carries a cookie session store under the name "sess", so handler code
// that calls sessions.Default(c) works unmodified. Apply Option values such
// as WithUser or WithCSPNonces for anything else the handler under test
// expects on the context.
func New(t testing.TB, method, target string, body io.Reader, opts ...Option) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()

	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(method, target, body)

	store := cookie.NewStore([]byte("ginctx-test-secret"))
	sessions.Sessions(sessionName, store)(c)

	for _, opt := range opts {
		opt(c)
	}

	return c, w
}
