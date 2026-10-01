package main

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service/accounts"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/fakesender"
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

// stdoutOf runs args and returns what the command printed to stdout.
func stdoutOf(t *testing.T, args []string) (string, error) {
	t.Helper()

	r, w, err := os.Pipe()
	require.NoError(t, err)

	orig := os.Stdout
	os.Stdout = w

	runErr := run(args)

	os.Stdout = orig
	require.NoError(t, w.Close())

	out, err := io.ReadAll(r)
	require.NoError(t, err)

	return string(out), runErr
}

func TestAdminLoginCode(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	user := testutil.Must(factory.User(ctx, db.DB))(t)
	args := []string{"admin", "login-code", "--database-url", db.URL, "--session-salt", "test-key", "--email", user.Email}

	_, err := stdoutOf(t, args)
	require.ErrorContains(t, err, "must first enter their email on the login page")

	svc := accounts.New(repo.New(db.DB), fakesender.New(), nil, accounts.WithCodeKey("test-key"))
	attemptID := testutil.Must(svc.StartLogin(ctx, user.Email, ""))(t)

	out, err := stdoutOf(t, args)
	require.NoError(t, err)
	require.Regexp(t, `^\d{6}\n$`, out, "only the code is printed")

	got, _, err := svc.FinishLogin(ctx, attemptID, strings.TrimSpace(out))
	require.NoError(t, err, "the printed code logs in to the user's attempt")
	require.Equal(t, user.ID, got.ID)
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
