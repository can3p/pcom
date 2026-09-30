package app_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/can3p/pcom/pkg/testutil/fakesender"
	"github.com/can3p/pcom/pkg/testutil/fakestorage"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/can3p/pcom/pkg/web/app"
	"github.com/stretchr/testify/require"
)

// newApp builds the whole router in process against a fresh test database,
// with the binary's real templates and a stub asset resolver.
func newApp(t *testing.T) http.Handler {
	t.Helper()

	db := testdb.New(t)

	return app.New(&app.Deps{
		DB:           db.DB,
		Sender:       fakesender.New(),
		MediaStorage: fakestorage.New(),
		Config: app.Config{
			HTMLDir:     "../../../cmd/web/client/html",
			SessionSalt: "test-salt",
			StaticAsset: func(n string) string { return "/static/" + n },
		},
	})
}

func TestRouter_Anonymous(t *testing.T) {
	t.Parallel()

	h := newApp(t)

	cases := []struct {
		name       string
		target     string
		wantStatus int
		// wantLoginReturn, when set, expects a redirect to /login carrying
		// it as the signed return_url; otherwise there is no redirect.
		wantLoginReturn string
		wantBody        string
	}{
		{name: "landing page renders", target: "/", wantStatus: http.StatusOK, wantBody: `href="/rss/public"`},
		{name: "controls redirect to login", target: "/controls/", wantStatus: http.StatusFound, wantLoginReturn: "/controls/"},
		{name: "non-UUID post id is not found", target: "/posts/not-a-uuid", wantStatus: http.StatusNotFound},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.target, nil))

			require.Equal(t, tc.wantStatus, rec.Code)
			require.Contains(t, rec.Body.String(), tc.wantBody)

			location := rec.Header().Get("Location")
			if tc.wantLoginReturn == "" {
				require.Empty(t, location)
				return
			}

			u, err := url.Parse(location)
			require.NoError(t, err)
			require.Equal(t, "/login", u.Path)
			require.Equal(t, tc.wantLoginReturn, u.Query().Get("return_url"))
			require.NotEmpty(t, u.Query().Get("sign"))
		})
	}
}
