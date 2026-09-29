package pgsession_test

import (
	"strings"
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

// TestHashPassword pins #119: new hashes are salted argon2id in PHC format,
// so the same password never hashes the same way twice and a leaked hash
// can't be matched against a precomputed table.
func TestHashPassword(t *testing.T) {
	t.Parallel()

	h1 := pgsession.HashPassword("s3cr3t")
	h2 := pgsession.HashPassword("s3cr3t")

	require.True(t, strings.HasPrefix(h1, "$argon2id$v=19$"), h1)
	require.NotEqual(t, h1, h2, "every hash gets its own salt")
}

func TestCheckUserPwd(t *testing.T) {
	t.Parallel()

	argon := pgsession.HashPassword("s3cr3t")

	cases := []struct {
		name       string
		stored     string
		email      string
		password   string
		ok, rehash bool
	}{
		{name: "argon2id match", stored: argon, email: "a@example.test", password: "s3cr3t", ok: true},
		{name: "argon2id ignores email", stored: argon, email: "other@example.test", password: "s3cr3t", ok: true},
		{name: "argon2id wrong password", stored: argon, email: "a@example.test", password: "wrong"},
		{name: "argon2id malformed", stored: "$argon2id$v=19$m=1,t=0,p=0$AA$AA", password: "s3cr3t"},
		{name: "legacy match asks for rehash", stored: pgsession.HashUserPwd("a@example.test", "s3cr3t"), email: "a@example.test", password: "s3cr3t", ok: true, rehash: true},
		{name: "legacy hashed mixed case, checked with that spelling", stored: pgsession.HashUserPwd("Bob@Example.test", "s3cr3t"), email: "Bob@Example.test", password: "s3cr3t", ok: true, rehash: true},
		{name: "legacy is checked against the given spelling only", stored: pgsession.HashUserPwd("Bob@Example.test", "s3cr3t"), email: "bob@example.test", password: "s3cr3t"},
		{name: "legacy wrong password", stored: pgsession.HashUserPwd("a@example.test", "s3cr3t"), email: "a@example.test", password: "wrong"},
		{name: "empty hash", stored: "", email: "a@example.test", password: ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ok, rehash := pgsession.CheckUserPwd(tc.stored, tc.email, tc.password)
			require.Equal(t, tc.ok, ok)
			require.Equal(t, tc.rehash, rehash)
		})
	}
}
