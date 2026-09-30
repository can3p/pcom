package main

import (
	"testing"

	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
)

func count(t *testing.T, db *sqlx.DB, query string) (n int) {
	t.Helper()
	require.NoError(t, db.Get(&n, query))

	return n
}

func TestAdminInvite(t *testing.T) {
	db := testdb.New(t)
	_, err := db.DB.Exec(`INSERT INTO users (id, email, username, timezone) VALUES ('00000000-0000-4000-8000-000000000001', 'a@pcom.test', 'a', 'UTC')`)
	require.NoError(t, err)

	require.NoError(t, run([]string{"admin", "invite", "--database-url", db.URL, "--email", "a@pcom.test", "--num", "3"}))
	require.Equal(t, 3, count(t, db.DB, `SELECT count(*) FROM user_invitations WHERE user_id = '00000000-0000-4000-8000-000000000001'`))

	require.ErrorContains(t, run([]string{"admin", "invite", "--database-url", db.URL, "--email", "nobody@pcom.test", "--num", "1"}), "nobody@pcom.test")
	require.ErrorContains(t, run([]string{"admin", "invite", "--database-url", db.URL, "--email", "a@pcom.test"}), "num")
}

func TestAdminRegistration(t *testing.T) {
	db := testdb.New(t)

	open := func() bool {
		return count(t, db.DB, `SELECT count(*) FROM system_settings WHERE registration_open`) > 0
	}

	require.NoError(t, run([]string{"admin", "registration", "--database-url", db.URL, "--close"}))
	require.False(t, open())
	require.NoError(t, run([]string{"admin", "registration", "--database-url", db.URL, "--open"}))
	require.True(t, open())

	require.Error(t, run([]string{"admin", "registration", "--database-url", db.URL}))
	require.Error(t, run([]string{"admin", "registration", "--database-url", db.URL, "--open", "--close"}))
}

func TestSeed(t *testing.T) {
	db := testdb.New(t)

	t.Setenv("FLY_APP_NAME", "")
	require.NoError(t, run([]string{"seed", "--database-url", db.URL, "--site-root", "http://site.test", "--reset"}))
	require.Equal(t, 5, count(t, db.DB, `SELECT count(*) FROM users`))

	t.Setenv("FLY_APP_NAME", "pcom-prod")
	require.ErrorContains(t, run([]string{"seed", "--database-url", db.URL, "--reset"}), "FLY_APP_NAME")
}

func TestCommands_MissingDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")

	for _, args := range [][]string{{"seed"}, {"admin", "invite", "--email", "a@b.c", "--num", "1"}, {"admin", "registration", "--open"}} {
		require.ErrorContains(t, run(args), "$DATABASE_URL", args)
	}
}
