package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/can3p/pcom/pkg/testutil/golden"
	"github.com/jessevdk/go-flags"
	"github.com/stretchr/testify/require"
)

// TestHelp pins every command's options, flags and environment variables:
// `web <command> --help` is the configuration reference, so a renamed or
// dropped setting shows up here.
func TestHelp(t *testing.T) {
	for _, args := range [][]string{
		{"--help"}, {"serve", "--help"}, {"seed", "--help"}, {"admin", "--help"}, {"admin", "invite", "--help"},
		{"admin", "registration", "--help"}, {"debug", "--help"}, {"debug", "feed", "--help"},
	} {
		name := strings.Join(args[:len(args)-1], "_") + "_help"
		if len(args) == 1 {
			name = "help"
		}

		t.Run(name, func(t *testing.T) {
			err := run(args)

			var flagsErr *flags.Error
			require.True(t, errors.As(err, &flagsErr) && flagsErr.Type == flags.ErrHelp, "want help, got %v", err)
			golden.Assert(t, name, []byte(flagsErr.Message))
		})
	}
}

// TestServe_MissingSettingsFailBeforeStarting checks that serve names every
// required setting of the parser at startup, before it opens the database.
func TestServe_MissingSettingsFailBeforeStarting(t *testing.T) {
	var vars []string

	for env, o := range serveOptions(t) {
		if o.Required {
			vars = append(vars, env)
			t.Setenv(env, "")
		}
	}

	require.Contains(t, vars, "USER_MEDIA_ENDPOINT")
	require.Contains(t, vars, "MJ_APIKEY_PUBLIC")

	err := run([]string{"serve"})

	var flagsErr *flags.Error
	require.True(t, errors.As(err, &flagsErr) && flagsErr.Type == flags.ErrRequired, "want a missing-settings error, got %v", err)

	for _, env := range vars {
		require.Contains(t, flagsErr.Message, "$"+env)
	}
}
