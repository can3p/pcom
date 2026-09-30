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

// parseCmd parses `name args...` into cmd with exactly env set.
func parseCmd(t *testing.T, name string, cmd flags.Commander, env map[string]string, args ...string) error {
	t.Helper()

	for _, k := range []string{"DATABASE_URL", "SITE_ROOT", "FLY_APP_NAME"} {
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
	got := &seedOnly{}
	require.NoError(t, parseCmd(t, "seed", got, map[string]string{"DATABASE_URL": "postgres://db/pcom"}, "--reset"))
	require.True(t, got.Reset)
}

func TestAdmin_Flags(t *testing.T) {
	db := map[string]string{"DATABASE_URL": "postgres://db/pcom"}

	inv := &inviteOnly{}
	require.NoError(t, parseCmd(t, "invite", inv, db, "--email", "a@pcom.test", "--num", "4"))
	require.Equal(t, "a@pcom.test", inv.Email)
	require.Equal(t, 4, inv.Num)
	require.ErrorContains(t, parseCmd(t, "invite", &inviteOnly{}, db, "--email", "a@pcom.test"), "num")
	require.ErrorContains(t, parseCmd(t, "invite", &inviteOnly{}, db, "--num", "1"), "email")
	require.ErrorContains(t, parseCmd(t, "invite", &inviteOnly{}, nil, "--email", "a@pcom.test", "--num", "1"), "$DATABASE_URL")

	reg := &registrationOnly{}
	require.NoError(t, parseCmd(t, "registration", reg, db, "--open"))
	require.True(t, reg.Open)
	require.False(t, reg.Close)
	require.ErrorContains(t, parseCmd(t, "registration", &registrationOnly{}, nil, "--open"), "$DATABASE_URL")
}
