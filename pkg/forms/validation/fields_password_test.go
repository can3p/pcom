package validation

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidatePassword(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		password string
		valid    bool
		desc     string
	}{
		// Invalid passwords
		{"", false, "empty password"},
		{"short", false, "too short (5 chars)"},
		{"1234567", false, "too short (7 chars)"},
		{"       ", false, "only spaces"},
		{" short ", false, "spaces don't count (5 non-space chars)"},

		// Valid passwords
		{"12345678", true, "exactly 8 characters"},
		{"password", true, "common password"},
		{"Pass@word!", true, "password with special chars"},
		{"very long password with spaces and numbers 123", true, "long password"},
		{"a b c d e f", true, "password with spaces (11 total)"},
		{"        secretpassword", true, "password with leading spaces"},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			err := ValidatePassword(tc.password)
			if tc.valid {
				require.NoError(t, err, "password: %q", tc.password)
			} else {
				require.Error(t, err, "password: %q", tc.password)
			}
		})
	}
}
