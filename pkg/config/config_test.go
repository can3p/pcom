package config_test

import (
	"fmt"
	"log/slog"
	"maps"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/can3p/gogo/settings"
	"github.com/can3p/pcom/pkg/config"
	"github.com/can3p/pcom/pkg/service/accounts"
	"github.com/jessevdk/go-flags"
	"github.com/stretchr/testify/require"
)

// serveOnly records the parsed settings instead of serving.
type serveOnly struct {
	config.Serve
}

func (*serveOnly) Execute([]string) error { return nil }

func newServeParser() (*flags.Parser, *serveOnly) {
	p := config.NewParser()
	cmd := &serveOnly{}

	if _, err := p.AddCommand("serve", "", "", cmd); err != nil {
		panic(err)
	}

	return p, cmd
}

// envKeys lists every environment variable the serve settings read.
func envKeys(t *testing.T) map[string]*flags.Option {
	t.Helper()

	p, _ := newServeParser()
	keys := map[string]*flags.Option{}

	var walk func(g *flags.Group)
	walk = func(g *flags.Group) {
		for _, o := range g.Options() {
			require.NotEmpty(t, o.EnvKeyWithNamespace(), "--%s has no env: tag", o.LongNameWithNamespace())
			keys[o.EnvKeyWithNamespace()] = o
		}

		for _, sub := range g.Groups() {
			walk(sub)
		}
	}
	walk(p.Find("serve").Group)

	return keys
}

// required is the smallest environment serve starts with.
var required = map[string]string{
	"DATABASE_URL":      "postgres://db/pcom",
	"SITE_ROOT":         "https://pcom.test",
	"SESSION_SALT":      "salt-value",
	"SENDER_ADDRESS":    "pcom@pcom.test",
	"MJ_APIKEY_PUBLIC":  "mj-public",
	"MJ_APIKEY_PRIVATE": "mj-private-value",

	"USER_MEDIA_ENDPOINT": "http://tommy:9000",
	"USER_MEDIA_BUCKET":   "pcom-media",
	"USER_MEDIA_REGION":   "eu-west-1",
	"USER_MEDIA_KEY":      "media-key",
	"USER_MEDIA_SECRET":   "media-secret",
}

// parseServe parses `serve` with exactly env set: every other variable the
// settings read is unset.
func parseServe(t *testing.T, env map[string]string, args ...string) (*config.Serve, error) {
	t.Helper()

	for k := range envKeys(t) {
		t.Setenv(k, "") // restores the old value when the test ends
		require.NoError(t, os.Unsetenv(k))
	}

	for k, v := range env {
		t.Setenv(k, v)
	}

	p, cmd := newServeParser()
	_, err := settings.Parse(p, append([]string{"serve"}, args...))

	return &cmd.Serve, err
}

func with(extra map[string]string) map[string]string {
	env := maps.Clone(required)
	maps.Copy(env, extra)

	return env
}

func TestServe_EveryVariable(t *testing.T) {
	type row struct {
		env   string
		value string
		get   func(*config.Serve) any
		def   any // with the variable unset; nil for required variables
		want  any
	}

	rows := []row{
		{"DATABASE_URL", "postgres://other/db", func(s *config.Serve) any { return s.Database.URL }, nil, "postgres://other/db"},
		{"SITE_ROOT", "https://other.test", func(s *config.Serve) any { return s.Web.SiteRoot }, nil, "https://other.test"},
		{"SESSION_SALT", "other-salt", func(s *config.Serve) any { return s.Web.SessionSalt.Reveal() }, nil, "other-salt"},
		{"PORT", "9123", func(s *config.Serve) any { return s.Web.Port }, 8080, 9123},
		{"GIN_MODE", "release", func(s *config.Serve) any { return s.Web.GinMode }, "", "release"},
		{"HTML_DIR", "tpl", func(s *config.Serve) any { return s.Web.HTMLDir }, "client/html", "tpl"},
		{"FORCE_SIGNUP", "true", func(s *config.Serve) any { return s.Web.ForceSignup.On() }, false, true},
		{"STATIC_CDN", "https://cdn.test", func(s *config.Serve) any { return s.Web.StaticCDN }, "", "https://cdn.test"},
		{"SECURE_COOKIES", "false", func(s *config.Serve) any { return s.Web.SecureCookies.On() }, true, false},
		{"HSTS", "false", func(s *config.Serve) any { return s.Web.HSTS.On() }, true, false},
		{"STATIC_CACHE", "false", func(s *config.Serve) any { return s.Web.StaticCache.On() }, true, false},
		{"MEDIA_PERMA_CACHE", "false", func(s *config.Serve) any { return s.Web.MediaPermaCache.On() }, true, false},
		{"SHOW_ERRORS", "true", func(s *config.Serve) any { return s.Web.ShowErrors.On() }, false, true},
		{"REPORT_PANICS", "false", func(s *config.Serve) any { return s.Web.ReportPanics.On() }, true, false},
		{"LOG_LEVEL", "debug", func(s *config.Serve) any { return s.Web.SlogLevel() }, slog.LevelInfo, slog.LevelDebug},
		{"ENABLE_PPROF", "true", func(s *config.Serve) any { return s.Web.EnablePprof.On() }, false, true},
		{"PAGE_SIZE", "10", func(s *config.Serve) any { return s.Limits.PageSize }, 30, 10},
		{"RSS_LIMIT", "20", func(s *config.Serve) any { return s.Limits.RSSLimit }, 50, 20},
		{"SENDER_ADDRESS", "other@pcom.test", func(s *config.Serve) any { return s.Mail.SenderAddress }, nil, "other@pcom.test"},
		{"ADMIN_ADDRESS", "admin@pcom.test", func(s *config.Serve) any { return s.Mail.AdminAddress }, "", "admin@pcom.test"},
		{"EMAIL_POLL_INTERVAL", "250ms", func(s *config.Serve) any { return s.Mail.PollInterval }, 10 * time.Second, 250 * time.Millisecond},
		{"MJ_APIKEY_PUBLIC", "other-public", func(s *config.Serve) any { return s.Mail.Mailjet.ApiKeyPublic }, nil, "other-public"},
		{"MJ_APIKEY_PRIVATE", "other-private", func(s *config.Serve) any { return s.Mail.Mailjet.ApiKeyPrivate.Reveal() }, nil, "other-private"},
		{"MJ_API_BASE", "http://tommy:8822", func(s *config.Serve) any { return s.Mail.Mailjet.BaseURL }, "", "http://tommy:8822"},
		{"USER_MEDIA_ENDPOINT", "http://tommy:9000", func(s *config.Serve) any { return s.Media.Endpoint }, nil, "http://tommy:9000"},
		{"USER_MEDIA_BUCKET", "pcom-media", func(s *config.Serve) any { return s.Media.Bucket }, nil, "pcom-media"},
		{"USER_MEDIA_REGION", "eu-west-1", func(s *config.Serve) any { return s.Media.Region }, nil, "eu-west-1"},
		{"USER_MEDIA_KEY", "media-key", func(s *config.Serve) any { return s.Media.Key }, nil, "media-key"},
		{"USER_MEDIA_SECRET", "media-secret", func(s *config.Serve) any { return s.Media.Secret.Reveal() }, nil, "media-secret"},
		{"USER_MEDIA_PATH_STYLE", "true", func(s *config.Serve) any { return s.Media.PathStyle.On() }, false, true},
		{"PROFILE_ABOUT_MAX_LENGTH", "300", func(s *config.Serve) any { return s.Limits.ProfileAboutMaxLength }, accounts.DefaultProfileAboutMaxLength, 300},
		{"USER_MEDIA_CDN", "https://media.test", func(s *config.Serve) any { return s.Media.CDN }, "", "https://media.test"},
	}

	keys := envKeys(t)
	tested := map[string]bool{}

	for _, r := range rows {
		tested[r.env] = true

		t.Run(r.env, func(t *testing.T) {
			_, ok := keys[r.env]
			require.True(t, ok, "no setting reads $%s", r.env)

			got, err := parseServe(t, with(map[string]string{r.env: r.value}))
			require.NoError(t, err)
			require.Equal(t, r.want, r.get(got), "with $%s=%s", r.env, r.value)

			if r.def == nil {
				require.True(t, keys[r.env].Required, "$%s has no default, so it must be required", r.env)
				return
			}

			env := with(nil)
			delete(env, r.env)

			got, err = parseServe(t, env)
			require.NoError(t, err)
			require.Equal(t, r.def, r.get(got), "with $%s unset", r.env)
		})
	}

	var untested []string
	for k := range keys {
		if !tested[k] {
			untested = append(untested, k)
		}
	}
	sort.Strings(untested)
	require.Empty(t, untested, "every setting needs a row in this table")
}

func TestServe_RequiredVariables(t *testing.T) {
	for env, opt := range envKeys(t) {
		if !opt.Required {
			continue
		}

		t.Run(env, func(t *testing.T) {
			for _, value := range []string{"unset", ""} {
				vars := with(nil)
				if value == "unset" {
					delete(vars, env)
				} else {
					vars[env] = value
				}

				_, err := parseServe(t, vars)
				require.ErrorContains(t, err, "$"+env, "with $%s %s", env, value)
			}
		})
	}
}

func TestServe_SecretsDontPrint(t *testing.T) {
	got, err := parseServe(t, with(map[string]string{"USER_MEDIA_SECRET": "media-secret-value"}))
	require.NoError(t, err)

	for _, format := range []string{"%v", "%+v", "%#v", "%s"} {
		out := fmt.Sprintf(format, *got)
		for _, secret := range []string{"salt-value", "mj-private-value", "media-secret-value"} {
			require.NotContains(t, out, secret, format)
		}
	}
}

func TestSwitch(t *testing.T) {
	showErrors := func(s *config.Serve) bool { return s.Web.ShowErrors.On() }
	hsts := func(s *config.Serve) bool { return s.Web.HSTS.On() }

	cases := []struct {
		name string
		env  map[string]string
		args []string
		get  func(*config.Serve) bool
		want bool
		err  string
	}{
		{name: "flag alone turns it on", args: []string{"--show-errors"}, get: showErrors, want: true},
		{name: "flag turns a default-on switch off", args: []string{"--hsts=false"}, get: hsts, want: false},
		{name: "1 is true", env: map[string]string{"SHOW_ERRORS": "1"}, get: showErrors, want: true},
		{name: "0 is false", env: map[string]string{"HSTS": "0"}, get: hsts, want: false},
		{name: "empty is an error, not a default", env: map[string]string{"SHOW_ERRORS": ""}, err: "want true or false"},
		{name: "garbage is an error", env: map[string]string{"SHOW_ERRORS": "yes please"}, err: "want true or false"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseServe(t, with(tc.env), tc.args...)
			if tc.err != "" {
				require.ErrorContains(t, err, tc.err)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tc.want, tc.get(got))
		})
	}
}
