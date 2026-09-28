package pgsession_test

import (
	"testing"

	"github.com/can3p/pcom/pkg/pgsession"
	"github.com/stretchr/testify/require"
)

// TestHashUserPwd_GoldenValues pins HashUserPwd's output for fixed inputs.
// It exists to catch an accidental change of the hashing scheme (algorithm,
// separator, encoding): such a change would invalidate every stored
// password hash and silently log out every user on deploy.
func TestHashUserPwd_GoldenValues(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		email    string
		password string
		want     string
	}{
		{
			name:     "typical email and password",
			email:    "user@example.com",
			password: "correct horse battery staple",
			want:     "80e61398eb33f6c1aed02b088bb08ec2633d6afd198bf90d394f3f812b8da29e",
		},
		{
			name:     "both empty",
			email:    "",
			password: "",
			want:     "e7ac0786668e0ff0f02b62bd04f45ff636fd82db63b1104601c975dc005f3a67",
		},
		{
			name:     "empty password",
			email:    "a@b.com",
			password: "",
			want:     "aff57bdb6fd02dfcc230670192ebd78392f76e61d57538cea9cbb0054e3dc654",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := pgsession.HashUserPwd(tc.email, tc.password)

			require.Equal(t, tc.want, got)
			require.Len(t, got, 64)
		})
	}
}

// TestHashUserPwd_Deterministic guards against the hash depending on
// anything but its two inputs (e.g. time, randomness).
func TestHashUserPwd_Deterministic(t *testing.T) {
	t.Parallel()

	a := pgsession.HashUserPwd("someone@example.com", "hunter2")
	b := pgsession.HashUserPwd("someone@example.com", "hunter2")

	require.Equal(t, a, b)
}

// TestHashUserPwd_DistinguishesEmailPasswordBoundary pins the ":" separator
// behavior: swapping characters between email and password must not
// collide, since the two fields are simply concatenated.
func TestHashUserPwd_DistinguishesEmailPasswordBoundary(t *testing.T) {
	t.Parallel()

	a := pgsession.HashUserPwd("ab", "c")
	b := pgsession.HashUserPwd("a", "bc")

	require.NotEqual(t, a, b)
}
