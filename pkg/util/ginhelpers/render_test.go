package ginhelpers_test

import (
	"encoding/json"
	"html/template"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/can3p/pcom/pkg/testutil/ginctx"
	"github.com/can3p/pcom/pkg/util/ginhelpers"
	"github.com/friendsofgo/errors"
	"github.com/gin-gonic/gin"
	"github.com/samber/mo"
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

func TestHTML_Ok(t *testing.T) {
	w := httptest.NewRecorder()
	gin.SetMode(gin.TestMode)
	c, engine := gin.CreateTestContext(w)
	engine.SetHTMLTemplate(template.Must(template.New("greeting").Parse("hello {{.}}")))

	ginhelpers.HTML(c, "greeting", mo.Ok("world"))

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "hello world", w.Body.String())
}

func TestHTML_ErrorMapping(t *testing.T) {
	withoutFlyAppName(t)

	cases := []struct {
		name       string
		err        error
		wantStatus int
	}{
		{"not found", ginhelpers.ErrNotFound, http.StatusNotFound},
		{"forbidden", ginhelpers.ErrForbidden, http.StatusForbidden},
		{"bad request", ginhelpers.ErrBadRequest, http.StatusBadRequest},
		{"unmapped error", errors.Errorf("boom"), http.StatusInternalServerError},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, w := ginctx.New(t, http.MethodGet, "/", nil)

			ginhelpers.HTML[string](c, "greeting", mo.Err[string](tc.err))

			require.Equal(t, tc.wantStatus, w.Code)
			require.Equal(t, tc.err.Error(), w.Body.String())
		})
	}
}

func TestHTML_ErrorMapping_InCluster_HidesBody(t *testing.T) {
	t.Setenv("FLY_APP_NAME", "pcom-test")

	c, w := ginctx.New(t, http.MethodGet, "/", nil)

	ginhelpers.HTML[string](c, "greeting", mo.Err[string](ginhelpers.ErrNotFound))
	c.Writer.WriteHeaderNow() // gin defers c.Status; flush it to the recorder

	require.Equal(t, http.StatusNotFound, w.Code)
	require.Empty(t, w.Body.String())
}

func TestHTML_NeedsLogin_Redirects(t *testing.T) {
	withoutFlyAppName(t)

	c, w := ginctx.New(t, http.MethodGet, "/private", nil)

	ginhelpers.HTML[string](c, "greeting", mo.Err[string](ginhelpers.ErrNeedsLogin))

	require.Equal(t, http.StatusFound, w.Code)
	require.True(t, c.IsAborted())
	require.Contains(t, w.Header().Get("Location"), "/login")
}

func TestAPI_Ok(t *testing.T) {
	c, w := ginctx.New(t, http.MethodGet, "/", nil)

	ginhelpers.API(c, mo.Ok(map[string]string{"foo": "bar"}))

	require.Equal(t, http.StatusOK, w.Code)

	var body struct {
		Data map[string]string `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, "bar", body.Data["foo"])
}

func TestAPI_ErrorMapping(t *testing.T) {
	withoutFlyAppName(t)

	cases := []struct {
		name       string
		err        error
		wantStatus int
	}{
		{"not found", ginhelpers.ErrNotFound, http.StatusNotFound},
		{"forbidden", ginhelpers.ErrForbidden, http.StatusForbidden},
		{"bad request", ginhelpers.ErrBadRequest, http.StatusBadRequest},
		{"unmapped error", errors.Errorf("boom"), http.StatusInternalServerError},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, w := ginctx.New(t, http.MethodGet, "/", nil)

			ginhelpers.API[string](c, mo.Err[string](tc.err))

			require.Equal(t, tc.wantStatus, w.Code)

			var body struct {
				Errors []string `json:"errors"`
			}
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
			require.Equal(t, []string{tc.err.Error()}, body.Errors)
		})
	}
}

func TestAPI_ErrorMapping_InCluster_HidesBody(t *testing.T) {
	t.Setenv("FLY_APP_NAME", "pcom-test")

	c, w := ginctx.New(t, http.MethodGet, "/", nil)

	ginhelpers.API[string](c, mo.Err[string](ginhelpers.ErrForbidden))
	c.Writer.WriteHeaderNow() // gin defers c.Status; flush it to the recorder

	require.Equal(t, http.StatusForbidden, w.Code)
	require.Empty(t, w.Body.String())
}
