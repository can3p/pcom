package app_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
		HTMLDir:       "../../../cmd/web/client/html",
		SessionSalt:   "test-salt",
		SenderAddress: "sender@pcom.test",
		AdminAddress:  "admin@pcom.test",
		StaticAsset:   func(n string) string { return "/static/" + n },
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

			if !on {
				require.Empty(t, snd.Sent())
				return
			}

			require.Len(t, snd.Sent(), 1)
			got := snd.Sent()[0]
			require.Equal(t, "panic_notification", got.EmailType)
			require.Equal(t, cfg.SenderAddress, got.Mail.From.Address)
			require.Len(t, got.Mail.To, 1)
			require.Equal(t, cfg.AdminAddress, got.Mail.To[0].Address)
		})
	}
}

// StaticCache makes served /static files immutable; off, they are not. Not
// parallel: /static is served from ./dist, so the test changes directory.
func TestRouter_StaticCache(t *testing.T) {
	htmlDir, err := filepath.Abs(settingsConfig().HTMLDir)
	require.NoError(t, err)

	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "dist"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "dist", "app.js"), []byte("x"), 0o644))
	t.Chdir(dir)

	for _, on := range []bool{true, false} {
		cfg := settingsConfig()
		cfg.HTMLDir = htmlDir
		cfg.StaticCache = on
		h := app.New(&app.Deps{DB: testdb.New(t).DB, Sender: fakesender.New(), MediaStorage: fakestorage.New(), Config: cfg})

		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/static/app.js", nil))
		require.Equal(t, http.StatusOK, rec.Code)
		require.Equal(t, on, strings.Contains(rec.Header().Get("Cache-Control"), "immutable"), "on=%v", on)
	}
}

// A missing file must not be cached forever.
func TestRouter_StaticCache_MissingFileNotImmutable(t *testing.T) {
	cfg := settingsConfig()
	htmlDir, err := filepath.Abs(cfg.HTMLDir)
	require.NoError(t, err)

	t.Chdir(t.TempDir())

	cfg.HTMLDir = htmlDir
	cfg.StaticCache = true
	h := app.New(&app.Deps{DB: testdb.New(t).DB, Sender: fakesender.New(), MediaStorage: fakestorage.New(), Config: cfg})

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/static/nothing-here", nil))
	require.Equal(t, http.StatusNotFound, rec.Code)
	require.NotContains(t, rec.Header().Get("Cache-Control"), "immutable")
}

// ForceOpenRegistration shows the signup form although system settings close
// registration; off, the page is the waiting list.
func TestRouter_ForceOpenRegistration(t *testing.T) {
	t.Parallel()

	for _, force := range []bool{true, false} {
		t.Run(map[bool]string{true: "on", false: "off"}[force], func(t *testing.T) {
			t.Parallel()

			db := testdb.New(t).DB
			_, err := db.Exec(`UPDATE system_settings SET registration_open = false`)
			require.NoError(t, err)

			cfg := settingsConfig()
			cfg.ForceOpenRegistration = force
			h := app.New(&app.Deps{DB: db, Sender: fakesender.New(), MediaStorage: fakestorage.New(), Config: cfg})

			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/signup", nil))
			require.Equal(t, http.StatusOK, rec.Code)
			require.Equal(t, force, strings.Contains(rec.Body.String(), "New Account"))
			require.Equal(t, !force, strings.Contains(rec.Body.String(), "Join the waiting list"))
		})
	}
}
