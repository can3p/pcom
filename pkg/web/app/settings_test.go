package app_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/can3p/pcom/pkg/testutil/fakesender"
	"github.com/can3p/pcom/pkg/testutil/fakestorage"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/can3p/pcom/pkg/web/app"
	"github.com/stretchr/testify/require"
)

func settingsConfig() app.Config {
	return app.Config{
		HTMLDir:     "../../../cmd/web/client/html",
		SessionSalt: "test-salt",
		StaticAsset: func(n string) string { return "/static/" + n },
	}
}

// Each behavior setting changes the response in its own way, on and off.
func TestRouter_Settings(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		set    func(*app.Config)
		target string
		// check gets the response and whether the setting is on.
		check func(t *testing.T, rec *httptest.ResponseRecorder, on bool)
	}{
		{
			name: "SecureCookies marks the session cookie Secure", set: func(c *app.Config) { c.SecureCookies = true },
			target: "/login",
			check: func(t *testing.T, rec *httptest.ResponseRecorder, on bool) {
				for _, ck := range rec.Result().Cookies() {
					if ck.Name == "sess" {
						require.Equal(t, on, ck.Secure)
						return
					}
				}
				t.Fatal("no session cookie")
			},
		},
		{
			name: "HSTS sends Strict-Transport-Security", set: func(c *app.Config) { c.HSTS = true },
			target: "/",
			check: func(t *testing.T, rec *httptest.ResponseRecorder, on bool) {
				require.Equal(t, on, rec.Header().Get("Strict-Transport-Security") != "")
			},
		},
		{
			name: "StaticCache makes /static immutable", set: func(c *app.Config) { c.StaticCache = true },
			target: "/static/nothing-here",
			check: func(t *testing.T, rec *httptest.ResponseRecorder, on bool) {
				require.Equal(t, on, strings.Contains(rec.Header().Get("Cache-Control"), "immutable"))
			},
		},
		{
			name: "ShowErrors shows error details", set: func(c *app.Config) { c.ShowErrors = true },
			target: "/users/nobody-here",
			check: func(t *testing.T, rec *httptest.ResponseRecorder, on bool) {
				require.Equal(t, http.StatusNotFound, rec.Code)
				require.Equal(t, on, rec.Body.String() == "not found")
			},
		},
	} {
		for _, on := range []bool{true, false} {
			name := tc.name + " off"
			if on {
				name = tc.name + " on"
			}

			t.Run(name, func(t *testing.T) {
				t.Parallel()

				cfg := settingsConfig()
				if on {
					tc.set(&cfg)
				}

				h := app.New(&app.Deps{DB: testdb.New(t).DB, Sender: fakesender.New(), MediaStorage: fakestorage.New(), Config: cfg})

				rec := httptest.NewRecorder()
				h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.target, nil))
				tc.check(t, rec, on)
			})
		}
	}
}

// ReportPanics mails the admin about a failed page; off, nothing is queued.
func TestRouter_ReportPanics(t *testing.T) {
	t.Parallel()

	for _, on := range []bool{true, false} {
		t.Run(map[bool]string{true: "on", false: "off"}[on], func(t *testing.T) {
			t.Parallel()

			cfg := settingsConfig()
			cfg.ReportPanics = on

			// a closed database makes the user styles page panic on its query
			db := testdb.New(t).DB
			snd := fakesender.New()
			h := app.New(&app.Deps{DB: db, Sender: snd, MediaStorage: fakestorage.New(), Config: cfg})
			require.NoError(t, db.Close())

			req := httptest.NewRequest(http.MethodGet, "/users/nobody/user_styles", nil)
			req.Header.Set("Referer", "http://example.com/")
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			require.Equal(t, on, len(snd.Sent()) == 1)
		})
	}
}
