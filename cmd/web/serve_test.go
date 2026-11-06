package main

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/can3p/gogo/settings"
	"github.com/can3p/pcom/pkg/abuse"
	"github.com/can3p/pcom/pkg/config"
	"github.com/can3p/pcom/pkg/emailaddr"
	"github.com/can3p/pcom/pkg/service/subscriptions"
	"github.com/can3p/pcom/pkg/testutil/fakestorage"
	"github.com/gin-gonic/gin"
	"github.com/jessevdk/go-flags"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
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
		{"SESSION_MAX_AGE=48h", "SessionMaxAge", 48 * time.Hour},
		{"HSTS=false", "HSTS", false},
		{"STATIC_CACHE=false", "StaticCache", false},
		{"SHOW_ERRORS=true", "ShowErrors", true},
		{"REPORT_PANICS=false", "ReportPanics", false},
		{"MAX_BODY_SIZE=2048", "MaxBodySize", int64(2048)},
		{"MAX_UPLOAD_SIZE=4096", "MaxUploadSize", int64(4096)},
		{"MAX_IMPORT_SIZE=8192", "MaxImportSize", int64(8192)},
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
			// Options are functions, which never compare equal; their mapping
			// is TestSubscriptionsOptions.
			got.Subscriptions, want.Subscriptions = nil, nil
			got.Abuse, want.Abuse = nil, nil
			require.Equal(t, want, got, "%s changed another field", r.set)
		})
	}
}

// restoreProcessSettings restores the process-wide settings the test changes.
func restoreProcessSettings(t *testing.T) {
	t.Helper()

	mode := gin.Mode()
	t.Cleanup(func() { gin.SetMode(mode) })
}

func TestNewLogger_Level(t *testing.T) {
	for _, r := range []struct {
		level   string
		enabled []zapcore.Level
		dropped []zapcore.Level
	}{
		{"debug", []zapcore.Level{zapcore.DebugLevel, zapcore.InfoLevel}, nil},
		{"info", []zapcore.Level{zapcore.InfoLevel, zapcore.ErrorLevel}, []zapcore.Level{zapcore.DebugLevel}},
		{"warn", []zapcore.Level{zapcore.WarnLevel}, []zapcore.Level{zapcore.DebugLevel, zapcore.InfoLevel}},
		{"error", []zapcore.Level{zapcore.ErrorLevel}, []zapcore.Level{zapcore.WarnLevel}},
	} {
		t.Run(r.level, func(t *testing.T) {
			cfg, err := parseServe(t, map[string]string{"LOG_LEVEL": r.level})
			require.NoError(t, err)

			logger := newLogger(cfg.Web, io.Discard)

			for _, l := range r.enabled {
				require.True(t, logger.Core().Enabled(l), "%s kept at %s", l, r.level)
			}

			for _, l := range r.dropped {
				require.False(t, logger.Core().Enabled(l), "%s dropped at %s", l, r.level)
			}
		})
	}
}

// json, the default, writes one JSON object per entry; console writes a
// line a person reads.
func TestNewLogger_Format(t *testing.T) {
	for _, r := range []struct {
		name string
		env  map[string]string
		json bool
	}{
		{"default", nil, true},
		{"json", map[string]string{"LOG_FORMAT": "json"}, true},
		{"console", map[string]string{"LOG_FORMAT": "console"}, false},
	} {
		t.Run(r.name, func(t *testing.T) {
			cfg, err := parseServe(t, r.env)
			require.NoError(t, err)

			var buf bytes.Buffer
			newLogger(cfg.Web, &buf).Info("hello", zap.String("k", "v"))

			var entry map[string]any
			if !r.json {
				require.Error(t, json.Unmarshal(buf.Bytes(), &entry))
				require.Contains(t, buf.String(), "hello")
				require.Contains(t, buf.String(), `{"k": "v"}`)
				return
			}

			require.NoError(t, json.Unmarshal(buf.Bytes(), &entry))
			require.Equal(t, "hello", entry["msg"])
			require.Equal(t, "info", entry["level"])
			require.Equal(t, "v", entry["k"])
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
			startPprof(enabled, addr, zap.NewNop())

			// the default mux the app could serve from never carries pprof
			_, pattern := http.DefaultServeMux.Handler(httptest.NewRequest(http.MethodGet, "/debug/pprof/", nil))
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
			req := httptest.NewRequest(http.MethodGet, "/media/thumb/a.jpg", nil)

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

func TestSubscriptionsOptions(t *testing.T) {
	baseSettings, err := parseServe(t, nil)
	require.NoError(t, err)

	limitsOf := func(cfg config.Serve) subscriptions.Limits {
		return subscriptions.New(nil, subscriptionsOptions(cfg.Subscriptions, nil)...).Limits()
	}
	base := limitsOf(baseSettings)

	// Each row changes one setting from the base; the value must land in the
	// named field and in no other.
	rows := []struct {
		set, field string
		want       any
	}{
		{"SUBSCRIBE_CONFIRM_LIFETIME=24h", "ConfirmLifetime", 24 * time.Hour},
		{"SUBSCRIBE_CONFIRMS_PER_ADDRESS=5", "ConfirmsPerAddress", 5},
		{"SUBSCRIBE_MANAGE_LINKS_PER_ADDRESS=6", "ManageLinksPerAddress", 6},
		{"SUBSCRIBE_CONFIRMS_PER_HOUR=7", "ConfirmsPerHour", 7},
		{"SUBSCRIBE_DOMAIN_LOOKUPS_PER_HOUR=8", "DomainLookupsPerHour", 8},
		{"SUBSCRIBE_PRUNE_EVERY=10m", "PruneEvery", 10 * time.Minute},
	}

	for _, r := range rows {
		t.Run(r.set, func(t *testing.T) {
			k, v, _ := strings.Cut(r.set, "=")

			cfg, err := parseServe(t, map[string]string{k: v})
			require.NoError(t, err)

			got, want := limitsOf(cfg), base

			require.Equal(t, r.want, reflect.ValueOf(got).FieldByName(r.field).Interface())

			reflect.ValueOf(&got).Elem().FieldByName(r.field).SetZero()
			reflect.ValueOf(&want).Elem().FieldByName(r.field).SetZero()
			require.Equal(t, want, got, "%s changed another field", r.set)
		})
	}
}

func TestAbuseOptions(t *testing.T) {
	type settings struct {
		Difficulty  int
		MaxAge      time.Duration
		MinFillTime time.Duration
	}

	settingsOf := func(cfg config.Serve) settings {
		g := abuse.New("salt", abuseOptions(cfg.Subscriptions)...)

		return settings{g.Difficulty(), g.MaxAge(), g.MinFillTime()}
	}

	baseSettings, err := parseServe(t, nil)
	require.NoError(t, err)

	base := settingsOf(baseSettings)
	require.Equal(t, settings{abuse.DefaultPowDifficulty, abuse.DefaultPowMaxAge, abuse.DefaultMinFillTime}, base)

	for _, r := range []struct {
		set, field string
		want       any
	}{
		{"SUBSCRIBE_POW_DIFFICULTY=12", "Difficulty", 12},
		{"SUBSCRIBE_POW_MAX_AGE=1h", "MaxAge", time.Hour},
		{"SUBSCRIBE_MIN_FILL_TIME=5s", "MinFillTime", 5 * time.Second},
	} {
		t.Run(r.set, func(t *testing.T) {
			k, v, _ := strings.Cut(r.set, "=")

			cfg, err := parseServe(t, map[string]string{k: v})
			require.NoError(t, err)

			got, want := settingsOf(cfg), base

			require.Equal(t, r.want, reflect.ValueOf(got).FieldByName(r.field).Interface())

			reflect.ValueOf(&got).Elem().FieldByName(r.field).SetZero()
			reflect.ValueOf(&want).Elem().FieldByName(r.field).SetZero()
			require.Equal(t, want, got, "%s changed another field", r.set)
		})
	}
}

// deadlineResolver records the deadline of the lookups it answers; every
// domain has a mail server.
type deadlineResolver struct {
	deadlines []time.Time
}

func (r *deadlineResolver) LookupMX(ctx context.Context, _ string) ([]*net.MX, error) {
	d, _ := ctx.Deadline()
	r.deadlines = append(r.deadlines, d)

	return []*net.MX{{Host: "mx.example.test.", Pref: 10}}, nil
}

func (r *deadlineResolver) LookupIPAddr(context.Context, string) ([]net.IPAddr, error) {
	return nil, nil
}

func TestNewEmailChecker_AllowDisposable(t *testing.T) {
	for value, want := range map[string]bool{"false": false, "true": true} {
		cfg, err := parseServe(t, map[string]string{"EMAIL_ALLOW_DISPOSABLE": value})
		require.NoError(t, err)
		require.Equal(t, want, newEmailChecker(cfg.Email).AllowsDisposable(), "EMAIL_ALLOW_DISPOSABLE=%q", value)
	}
}

func TestNewEmailChecker_MXCache(t *testing.T) {
	for _, r := range []struct {
		env map[string]string
		// after is how long after the first check the second one runs.
		after   time.Duration
		domains []string
		lookups int
	}{
		{map[string]string{}, 30 * time.Millisecond, []string{"a.test", "a.test"}, 1},
		{map[string]string{"EMAIL_MX_CACHE": "10ms"}, 30 * time.Millisecond, []string{"a.test", "a.test"}, 2},
		{map[string]string{"EMAIL_MX_CACHE": "0s"}, 0, []string{"a.test", "a.test"}, 2},
		{map[string]string{"EMAIL_MX_CACHE_SIZE": "1"}, 0, []string{"a.test", "b.test", "a.test"}, 3},
		{map[string]string{"EMAIL_MX_CACHE_SIZE": "2"}, 0, []string{"a.test", "b.test", "a.test"}, 2},
	} {
		cfg, err := parseServe(t, r.env)
		require.NoError(t, err)

		resolver := &deadlineResolver{}
		checker := newEmailChecker(cfg.Email, emailaddr.WithResolver(resolver))

		for i, domain := range r.domains {
			if i == 1 {
				time.Sleep(r.after)
			}

			_, err := checker.CheckDomain(context.Background(), domain)
			require.NoError(t, err)
		}

		require.Len(t, resolver.deadlines, r.lookups, "%v after %v", r.env, r.after)
	}
}

func TestNewEmailChecker_MXTimeout(t *testing.T) {
	for _, r := range []struct {
		value string
		want  time.Duration // 0: no lookup
	}{
		{"5s", 5 * time.Second},
		{"0s", 0},
	} {
		t.Run("EMAIL_MX_TIMEOUT="+r.value, func(t *testing.T) {
			cfg, err := parseServe(t, map[string]string{"EMAIL_MX_TIMEOUT": r.value})
			require.NoError(t, err)

			resolver := &deadlineResolver{}
			_, err = newEmailChecker(cfg.Email, emailaddr.WithResolver(resolver)).CheckDomain(context.Background(), "example.test")
			require.NoError(t, err)

			if r.want == 0 {
				require.Empty(t, resolver.deadlines, "the check ran although it is off")
				return
			}

			require.Len(t, resolver.deadlines, 1)
			require.WithinDuration(t, time.Now().Add(r.want), resolver.deadlines[0], time.Second)
		})
	}
}
