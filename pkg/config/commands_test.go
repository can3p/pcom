package config_test

import (
	"os"
	"testing"

	"github.com/can3p/gogo/settings"
	"github.com/can3p/pcom/pkg/config"
	"github.com/jessevdk/go-flags"
	"github.com/stretchr/testify/require"
)

type seedOnly struct{ config.Seed }

func (*seedOnly) Execute([]string) error { return nil }

type inviteOnly struct{ config.AdminInvite }

func (*inviteOnly) Execute([]string) error { return nil }

type registrationOnly struct{ config.AdminRegistration }

func (*registrationOnly) Execute([]string) error { return nil }

type loginCodeOnly struct{ config.AdminLoginCode }

func (*loginCodeOnly) Execute([]string) error { return nil }

// parseCmd parses `name args...` into cmd with exactly env set.
func parseCmd(t *testing.T, name string, cmd flags.Commander, env map[string]string, args ...string) error {
	t.Helper()

	for _, k := range []string{"DATABASE_URL", "SITE_ROOT", "FLY_APP_NAME", "SESSION_SALT"} {
		t.Setenv(k, "")
		require.NoError(t, os.Unsetenv(k))
	}

	for k, v := range env {
		t.Setenv(k, v)
	}

	p := config.NewParser()
	_, err := p.AddCommand(name, "", "", cmd)
	require.NoError(t, err)

	_, err = settings.Parse(p, append([]string{name}, args...))

	return err
}

func TestSeed_EveryVariable(t *testing.T) {
	rows := []struct {
		env, value string
		get        func(*config.Seed) any
		def, want  any
	}{
		{"DATABASE_URL", "postgres://other/db", func(s *config.Seed) any { return s.Database.URL }, nil, "postgres://other/db"},
		{"SITE_ROOT", "https://other.test", func(s *config.Seed) any { return s.SiteRoot }, "", "https://other.test"},
		{"FLY_APP_NAME", "pcom-prod", func(s *config.Seed) any { return s.Production }, "", "pcom-prod"},
	}

	for _, r := range rows {
		t.Run(r.env, func(t *testing.T) {
			base := map[string]string{"DATABASE_URL": "postgres://db/pcom"}

			got := &seedOnly{}
			env := map[string]string{"DATABASE_URL": "postgres://db/pcom", r.env: r.value}
			require.NoError(t, parseCmd(t, "seed", got, env))
			require.Equal(t, r.want, r.get(&got.Seed))

			if r.def == nil {
				require.ErrorContains(t, parseCmd(t, "seed", &seedOnly{}, map[string]string{}), "$"+r.env)
				return
			}

			got = &seedOnly{}
			require.NoError(t, parseCmd(t, "seed", got, base))
			require.Equal(t, r.def, r.get(&got.Seed))
		})
	}
}

func TestSeed_Reset(t *testing.T) {
	db := map[string]string{"DATABASE_URL": "postgres://db/pcom"}

	for _, tc := range []struct {
		args []string
		want bool
	}{{[]string{"--reset"}, true}, {nil, false}} {
		got := &seedOnly{}
		require.NoError(t, parseCmd(t, "seed", got, db, tc.args...))
		require.Equal(t, tc.want, got.Reset, tc.args)
	}
}

// Only what the command tests in cmd/web do not cover: the required --email
// and the --close flag.
func TestAdmin_Flags(t *testing.T) {
	db := map[string]string{"DATABASE_URL": "postgres://db/pcom"}

	require.ErrorContains(t, parseCmd(t, "invite", &inviteOnly{}, db, "--num", "1"), "email")

	reg := &registrationOnly{}
	require.NoError(t, parseCmd(t, "registration", reg, db, "--close"))
	require.True(t, reg.Close)
	require.False(t, reg.Open)
}

func TestAdminLoginCode_Flags(t *testing.T) {
	db := map[string]string{"DATABASE_URL": "postgres://db/pcom"}

	require.ErrorContains(t, parseCmd(t, "login-code", &loginCodeOnly{}, db, "--email", "a@b.c"), "SESSION_SALT")
	require.ErrorContains(t, parseCmd(t, "login-code", &loginCodeOnly{}, map[string]string{"DATABASE_URL": "postgres://db/pcom", "SESSION_SALT": "s"}), "email")

	got := &loginCodeOnly{}
	require.NoError(t, parseCmd(t, "login-code", got, map[string]string{"DATABASE_URL": "postgres://db/pcom", "SESSION_SALT": "s"}, "--email", "a@b.c"))
	require.Equal(t, "a@b.c", got.Email)
	require.Equal(t, "s", got.SessionSalt.Reveal())
}
