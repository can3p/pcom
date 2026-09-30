package app_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/can3p/pcom/pkg/testutil/fakesender"
	"github.com/can3p/pcom/pkg/testutil/fakestorage"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/can3p/pcom/pkg/web/app"
	"github.com/stretchr/testify/require"
)

// Production hides the text of a failed page, development shows it.
func TestRouter_ErrorTextOnlyOutsideCluster(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name      string
		inCluster bool
		wantBody  string
	}{
		{name: "development shows the error", inCluster: false, wantBody: "not found"},
		{name: "production hides the error", inCluster: true, wantBody: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := app.New(&app.Deps{
				DB:           testdb.New(t).DB,
				Sender:       fakesender.New(),
				MediaStorage: fakestorage.New(),
				Config: app.Config{
					HTMLDir:     "../../../cmd/web/client/html",
					SessionSalt: "test-salt",
					StaticAsset: func(n string) string { return "/static/" + n },
					InCluster:   tc.inCluster,
				},
			})

			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/users/nobody-here", nil))

			require.Equal(t, http.StatusNotFound, rec.Code)
			require.Equal(t, tc.wantBody, rec.Body.String())
		})
	}
}
