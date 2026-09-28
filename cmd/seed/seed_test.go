package main

import (
	"bytes"
	"context"
	"net/http"
	"testing"

	"github.com/can3p/pcom/pkg/auth"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/ginctx"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func options(reset bool) Options {
	return Options{Reset: reset, Getenv: func(string) string { return "" }, SiteRoot: "http://site.test"}
}

func count(t *testing.T, db *sqlx.DB, query string, args ...any) int {
	t.Helper()

	var n int

	require.NoError(t, db.QueryRow(query, args...).Scan(&n))

	return n
}

func userID(t *testing.T, db *sqlx.DB, name string) string {
	t.Helper()

	var id string

	require.NoError(t, db.QueryRow(`SELECT id FROM users WHERE username = $1`, name).Scan(&id))

	return id
}

func TestSeed_BuildsNamedWorld(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := testdb.New(t).DB

	var out bytes.Buffer

	require.NoError(t, Run(ctx, db.DB, &out, options(false)))

	// Users can log in with the real password check.
	assert.Equal(t, 5, count(t, db, `SELECT count(*) FROM users`))

	for _, name := range []string{"alice", "bob", "carol", "dave", "eve"} {
		c, _ := ginctx.New(t, http.MethodGet, "/", nil)
		require.NoError(t, auth.CheckCredentials(c, db, name+"@example.test", "password"), name)
	}

	alice, bob, carol, dave, eve := userID(t, db, "alice"), userID(t, db, "bob"), userID(t, db, "carol"), userID(t, db, "dave"), userID(t, db, "eve")

	for _, tc := range []struct {
		a, b string
		want bool
	}{{alice, bob, true}, {bob, carol, true}, {alice, carol, false}, {dave, alice, false}} {
		got, err := factory.ConnectionExists(ctx, db, tc.a, tc.b)
		require.NoError(t, err)
		assert.Equal(t, tc.want, got)
	}

	assert.Equal(t, 1, count(t, db, `SELECT count(*) FROM user_invitations WHERE user_id = $1 AND created_user_id IS NULL`, eve))

	for _, v := range []string{"direct_only", "second_degree", "public"} {
		assert.Equal(t, 1, count(t, db, `SELECT count(*) FROM posts WHERE visibility_radius = $1 AND published_at IS NOT NULL AND url_id IS NULL`, v), v)
	}

	assert.Equal(t, 1, count(t, db, `SELECT count(*) FROM posts WHERE published_at IS NULL`), "draft")
	assert.Equal(t, 1, count(t, db, `SELECT count(*) FROM posts WHERE url_id IS NOT NULL`), "url post")
	assert.Equal(t, 1, count(t, db, `SELECT count(*) FROM post_shares`))
	assert.Equal(t, 1, count(t, db, `SELECT count(*) FROM post_prompts WHERE dismissed_at IS NULL AND post_id IS NULL`), "open prompt")

	// Comment thread three levels deep.
	assert.Equal(t, 3, count(t, db, `SELECT count(*) FROM post_comments`))
	assert.Equal(t, 1, count(t, db, `SELECT count(*) FROM post_comments c3
		JOIN post_comments c2 ON c3.parent_comment_id = c2.id
		JOIN post_comments c1 ON c2.parent_comment_id = c1.id
		WHERE c1.parent_comment_id IS NULL AND c3.top_comment_id = c1.id`))

	keys, err := factory.ListAPIKeys(ctx, db, alice)
	require.NoError(t, err)
	require.Len(t, keys, 1)
	assert.Equal(t, AliceAPIKey, keys[0].APIKey)

	assert.Equal(t, 1, count(t, db, `SELECT count(*) FROM rss_feeds WHERE url LIKE 'https://example.test/%'`))
	assert.Equal(t, 3, count(t, db, `SELECT count(*) FROM rss_items`))

	assert.Equal(t, 1, count(t, db, `SELECT count(*) FROM user_connection_mediation_requests WHERE who_user_id = $1 AND target_user_id = $2`, alice, carol))
	assert.Equal(t, 0, count(t, db, `SELECT count(*) FROM user_connection_mediators`), "pending")
	assert.Equal(t, 1, count(t, db, `SELECT count(*) FROM system_settings WHERE registration_open`))

	summary := out.String()
	for _, want := range []string{"alice", "bob", "carol", "dave", "eve", "password", AliceAPIKey, "http://site.test/login", "public", "connections", "registered_users"} {
		assert.Contains(t, summary, want)
	}
}

func TestSeed_SecondRunFailsAndChangesNothing(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := testdb.New(t).DB

	require.NoError(t, Run(ctx, db.DB, &bytes.Buffer{}, options(false)))

	before := count(t, db, `SELECT count(*) FROM posts`)

	var out bytes.Buffer

	require.Error(t, Run(ctx, db.DB, &out, options(false)))
	assert.Empty(t, out.String())
	assert.Equal(t, 5, count(t, db, `SELECT count(*) FROM users`))
	assert.Equal(t, before, count(t, db, `SELECT count(*) FROM posts`))
}

func TestSeed_ResetKeepsMigrationsAndSettings(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := testdb.New(t).DB

	require.NoError(t, Run(ctx, db.DB, &bytes.Buffer{}, options(false)))

	migrations := count(t, db, `SELECT count(*) FROM migrations`)
	require.Positive(t, migrations)

	_, err := db.Exec(`UPDATE system_settings SET registration_open = false`)
	require.NoError(t, err)

	require.NoError(t, Run(ctx, db.DB, &bytes.Buffer{}, options(true)))

	assert.Equal(t, migrations, count(t, db, `SELECT count(*) FROM migrations`))
	assert.Equal(t, 1, count(t, db, `SELECT count(*) FROM system_settings`))
	assert.Equal(t, 1, count(t, db, `SELECT count(*) FROM system_settings WHERE registration_open`))
	assert.Equal(t, 5, count(t, db, `SELECT count(*) FROM users`))
	assert.Equal(t, 1, count(t, db, `SELECT count(*) FROM user_api_keys`))
	assert.Equal(t, 3, count(t, db, `SELECT count(*) FROM post_comments`))
}

func TestSeed_RefusesOnFly(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	o := options(true)
	o.Getenv = func(k string) string {
		if k == "FLY_APP_NAME" {
			return "pcom-prod"
		}

		return ""
	}

	var out bytes.Buffer

	require.ErrorContains(t, Run(context.Background(), db.DB, &out, o), "FLY_APP_NAME")
	assert.Empty(t, out.String())
	assert.Equal(t, 0, count(t, db, `SELECT count(*) FROM users`))
}
