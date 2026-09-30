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
// missing required setting at startup, before it opens the database.
func TestServe_MissingSettingsFailBeforeStarting(t *testing.T) {
	for _, env := range []string{"DATABASE_URL", "SITE_ROOT", "SESSION_SALT", "SENDER_ADDRESS", "MJ_APIKEY_PUBLIC", "MJ_APIKEY_PRIVATE"} {
		t.Setenv(env, "")
	}

	err := run([]string{"serve"})

	var flagsErr *flags.Error
	require.True(t, errors.As(err, &flagsErr) && flagsErr.Type == flags.ErrRequired, "want a missing-settings error, got %v", err)

	for _, want := range []string{"$DATABASE_URL", "$SITE_ROOT", "$SESSION_SALT", "$SENDER_ADDRESS", "$MJ_APIKEY_PUBLIC", "$MJ_APIKEY_PRIVATE"} {
		require.Contains(t, flagsErr.Message, want)
	}
}
