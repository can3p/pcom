package main

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// unsetEnv makes sure the named environment variable is not set for the
// duration of the test, restoring whatever value (or absence) it had
// before.
func unsetEnv(t *testing.T, name string) {
	t.Helper()

	old, ok := os.LookupEnv(name)
	require.NoError(t, os.Unsetenv(name))

	t.Cleanup(func() {
		if ok {
			_ = os.Setenv(name, old)
		} else {
			_ = os.Unsetenv(name)
		}
	})
}

func TestEnforceEnvVars(t *testing.T) {
	unsetEnv(t, "PCOM_TEST_REQUIRED_VAR")

	require.Panics(t, func() {
		enforceEnvVars([]string{"PCOM_TEST_REQUIRED_VAR"})
	})

	t.Setenv("PCOM_TEST_REQUIRED_VAR", "set")

	require.NotPanics(t, func() {
		enforceEnvVars([]string{"PCOM_TEST_REQUIRED_VAR"})
	})
}

func TestEnforceEnvVars_NoVarsRequired(t *testing.T) {
	t.Parallel()

	require.NotPanics(t, func() {
		enforceEnvVars(nil)
	})
}
