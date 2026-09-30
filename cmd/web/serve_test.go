package main

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/can3p/gogo/settings"
	"github.com/can3p/pcom/pkg/config"
	"github.com/can3p/pcom/pkg/testutil/fakestorage"
	"github.com/gin-gonic/gin"
	"github.com/jessevdk/go-flags"
	"github.com/stretchr/testify/require"
)

// serveOptions is every option of the serve command, by environment variable.
func serveOptions(t *testing.T) map[string]*flags.Option {
	t.Helper()

	opts := map[string]*flags.Option{}

	var walk func(g *flags.Group)
	walk = func(g *flags.Group) {
		for _, o := range g.Options() {
			opts[o.EnvKeyWithNamespace()] = o
		}

		for _, sub := range g.Groups() {
			walk(sub)
		}
	}
	walk(newParser().Find("serve").Group)

	return opts
}

// parseServe parses `serve args` through the binary's own parser with env as
// the whole environment, and returns the settings it would serve with.
func parseServe(t *testing.T, env map[string]string, args ...string) (config.Serve, error) {
	t.Helper()

	for k, o := range serveOptions(t) {
		t.Setenv(k, "") // restores the old value when the test ends
		require.NoError(t, os.Unsetenv(k))

		if o.Required {
			t.Setenv(k, "required-"+strings.ToLower(k))
		}
	}

	for k, v := range env {
		t.Setenv(k, v)
	}

	var got config.Serve

	p := newParser()
	p.CommandHandler = func(cmd flags.Commander, _ []string) error {
		got = cmd.(*serveCmd).Serve
		return nil
	}

	_, err := settings.Parse(p, append([]string{"serve"}, args...))

	return got, err
}

func TestAppConfig(t *testing.T) {
	baseSettings, err := parseServe(t, nil)
	require.NoError(t, err)

	base := appConfig(baseSettings, nil)

	// Each row changes one setting from the base; the value must land in the
	// named field and in no other.
	rows := []struct {
		set, field string
		want       any
	}{
		{"SITE_ROOT=https://site.test", "SiteRoot", "https://site.test"},
		{"SESSION_SALT=salt-value", "SessionSalt", "salt-value"},
		{"STATIC_CDN=https://static.test", "StaticCDN", "https://static.test"},
		{"USER_MEDIA_CDN=https://media.test", "MediaCDN", "https://media.test"},
		{"SENDER_ADDRESS=sender@pcom.test", "SenderAddress", "sender@pcom.test"},
		{"ADMIN_ADDRESS=admin@pcom.test", "AdminAddress", "admin@pcom.test"},
		{"HTML_DIR=tpl-env", "HTMLDir", "tpl-env"},
		{"--html-dir=tpl-flag", "HTMLDir", "tpl-flag"},
		{"FORCE_SIGNUP=true", "ForceOpenRegistration", true},
		{"SECURE_COOKIES=false", "SecureCookies", false},
		{"HSTS=false", "HSTS", false},
		{"STATIC_CACHE=false", "StaticCache", false},
		{"SHOW_ERRORS=true", "ShowErrors", true},
		{"REPORT_PANICS=false", "ReportPanics", false},
	}

	for _, r := range rows {
		t.Run(r.set, func(t *testing.T) {
			env, args := map[string]string{}, []string(nil)

			if strings.HasPrefix(r.set, "--") {
				args = []string{r.set}
			} else {
				k, v, _ := strings.Cut(r.set, "=")
				env[k] = v
			}

			cfg, err := parseServe(t, env, args...)
			require.NoError(t, err)

			got, want := appConfig(cfg, nil), base

			require.Equal(t, r.want, reflect.ValueOf(got).FieldByName(r.field).Interface())

			reflect.ValueOf(&got).Elem().FieldByName(r.field).SetZero()
			reflect.ValueOf(&want).Elem().FieldByName(r.field).SetZero()
			require.Equal(t, want, got, "%s changed another field", r.set)
		})
	}
}

// restoreProcessSettings restores the process-wide settings the test changes.
func restoreProcessSettings(t *testing.T) {
	t.Helper()

	level, mode := slog.SetLogLoggerLevel(slog.LevelInfo), gin.Mode()
	t.Cleanup(func() {
		slog.SetLogLoggerLevel(level)
		gin.SetMode(mode)
	})
}

func TestApplyProcessSettings_LogLevel(t *testing.T) {
	for _, r := range []struct {
		level   string
		enabled []slog.Level
		dropped []slog.Level
	}{
		{"debug", []slog.Level{slog.LevelDebug, slog.LevelInfo}, nil},
		{"info", []slog.Level{slog.LevelInfo, slog.LevelError}, []slog.Level{slog.LevelDebug}},
		{"warn", []slog.Level{slog.LevelWarn}, []slog.Level{slog.LevelDebug, slog.LevelInfo}},
		{"error", []slog.Level{slog.LevelError}, []slog.Level{slog.LevelWarn}},
	} {
		t.Run(r.level, func(t *testing.T) {
			restoreProcessSettings(t)

			cfg, err := parseServe(t, map[string]string{"LOG_LEVEL": r.level})
			require.NoError(t, err)

			applyProcessSettings(cfg)

			for _, l := range r.enabled {
				require.True(t, slog.Default().Enabled(context.Background(), l), "%s kept at %s", l, r.level)
			}

			for _, l := range r.dropped {
				require.False(t, slog.Default().Enabled(context.Background(), l), "%s dropped at %s", l, r.level)
			}
		})
	}
}

func TestApplyProcessSettings_GinMode(t *testing.T) {
	for _, r := range []struct{ name, setting, want string }{
		{"release", gin.ReleaseMode, gin.ReleaseMode},
		{"debug", gin.DebugMode, gin.DebugMode},
		{"empty leaves gin alone", "", gin.TestMode},
	} {
		t.Run(r.name, func(t *testing.T) {
			restoreProcessSettings(t)
			gin.SetMode(gin.TestMode)

			cfg, err := parseServe(t, map[string]string{"GIN_MODE": r.setting})
			require.NoError(t, err)

			applyProcessSettings(cfg)
			require.Equal(t, r.want, gin.Mode())
		})
	}
}

func freeAddr(t *testing.T) string {
	t.Helper()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	require.NoError(t, l.Close())

	return l.Addr().String()
}

func TestStartPprof(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		t.Run(map[bool]string{true: "on", false: "off"}[enabled], func(t *testing.T) {
			orig := http.DefaultServeMux
			t.Cleanup(func() { http.DefaultServeMux = orig })

			addr := freeAddr(t)
			startPprof(enabled, addr)

			// the default mux the app could serve from never carries pprof
			_, pattern := http.DefaultServeMux.Handler(httptest.NewRequest("GET", "/debug/pprof/", nil))
			require.Empty(t, pattern)

			get := func() (int, error) {
				resp, err := http.Get("http://" + addr + "/debug/pprof/")
				if err != nil {
					return 0, err
				}

				_ = resp.Body.Close()

				return resp.StatusCode, nil
			}

			if enabled {
				require.Eventually(t, func() bool { code, err := get(); return err == nil && code == http.StatusOK }, 5*time.Second, 20*time.Millisecond)
				return
			}

			time.Sleep(200 * time.Millisecond)

			_, err := get()
			require.Error(t, err, "pprof is served although it is off")
		})
	}
}

func TestNewMediaServer_PermaCache(t *testing.T) {
	var img bytes.Buffer

	src := image.NewRGBA(image.Rect(0, 0, 8, 8))
	src.Set(1, 1, color.White)
	require.NoError(t, jpeg.Encode(&img, src, nil))

	outer, storage := t, fakestorage.New()
	require.NoError(t, storage.UploadFile(context.Background(), "a.jpg", img.Bytes(), "image/jpeg"))

	for _, r := range []struct{ value, wantCache string }{
		{"true", "max-age=604800"},
		{"false", ""},
	} {
		t.Run("MEDIA_PERMA_CACHE="+r.value, func(t *testing.T) {
			cfg, err := parseServe(t, map[string]string{"MEDIA_PERMA_CACHE": r.value})
			require.NoError(t, err)

			ms, cleanup, err := newMediaServer(storage, cfg.Web)
			require.NoError(t, err)

			outer.Cleanup(cleanup) // vips can't restart within a process, so shut it down after the last row

			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Params = gin.Params{{Key: "class", Value: "thumb"}}
			req := httptest.NewRequest("GET", "/media/thumb/a.jpg", nil)

			require.NoError(t, ms.ServeImage(c, ms, req, w, "a.jpg"))

			resp := w.Result()
			require.Equal(t, http.StatusOK, resp.StatusCode)

			_, _ = io.Copy(io.Discard, resp.Body)

			if r.wantCache == "" {
				require.Empty(t, resp.Header.Get("Cache-Control"))
			} else {
				require.Contains(t, resp.Header.Get("Cache-Control"), r.wantCache)
			}
		})
	}
}
