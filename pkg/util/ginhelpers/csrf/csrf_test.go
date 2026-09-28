package csrf_test

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/can3p/pcom/pkg/auth"
	"github.com/can3p/pcom/pkg/testutil/ginctx"
	"github.com/can3p/pcom/pkg/util/ginhelpers/csrf"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// setFormBody replaces the context's request body with an
// application/x-www-form-urlencoded encoding of vals and sets the matching
// content type, the way a real form POST (including the header_csrf hidden
// field) would arrive.
func setFormBody(c *gin.Context, vals url.Values) {
	c.Request.Body = io.NopCloser(strings.NewReader(vals.Encode()))
	c.Request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
}

func TestCheckCSRF_MissingToken(t *testing.T) {
	t.Parallel()

	c, w := ginctx.New(t, http.MethodPost, "/", nil)

	csrf.CheckCSRF(c)

	require.True(t, c.IsAborted())
	require.Equal(t, http.StatusForbidden, w.Code)
}

func TestCheckCSRF_WrongToken_Header(t *testing.T) {
	t.Parallel()

	c, w := ginctx.New(t, http.MethodPost, "/", nil)
	_ = auth.GetUserData(c) // seeds the session's csrf token

	c.Request.Header.Set("X-CSRFToken", "not-the-right-token")

	csrf.CheckCSRF(c)

	require.True(t, c.IsAborted())
	require.Equal(t, http.StatusForbidden, w.Code)
}

func TestCheckCSRF_WrongToken_FormField(t *testing.T) {
	t.Parallel()

	c, w := ginctx.New(t, http.MethodPost, "/", nil)
	_ = auth.GetUserData(c) // seeds the session's csrf token

	setFormBody(c, url.Values{"header_csrf": {"not-the-right-token"}})

	csrf.CheckCSRF(c)

	require.True(t, c.IsAborted())
	require.Equal(t, http.StatusForbidden, w.Code)
}

func TestCheckCSRF_RightToken_Header(t *testing.T) {
	t.Parallel()

	c, w := ginctx.New(t, http.MethodPost, "/", nil)
	token := auth.GetUserData(c).CSRFToken

	c.Request.Header.Set("X-CSRFToken", token)

	csrf.CheckCSRF(c)

	require.False(t, c.IsAborted())
	require.Equal(t, http.StatusOK, w.Code)
}

func TestCheckCSRF_RightToken_FormField(t *testing.T) {
	t.Parallel()

	c, w := ginctx.New(t, http.MethodPost, "/", nil)
	token := auth.GetUserData(c).CSRFToken

	setFormBody(c, url.Values{"header_csrf": {token}})

	csrf.CheckCSRF(c)

	require.False(t, c.IsAborted())
	require.Equal(t, http.StatusOK, w.Code)
}
