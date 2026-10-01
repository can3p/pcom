package seed

import (
	"bytes"
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service/accounts"
	"github.com/can3p/pcom/pkg/service/reading"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/ginctx"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func options(reset bool) Options {
	return Options{Reset: reset, SiteRoot: "http://site.test"}
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
		require.NoError(t, accounts.New(repo.New(db), nil, nil).CheckCredentials(c, name+"@example.test", "password"), name)
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
		assert.Equal(t, 1, count(t, db, `SELECT count(*) FROM posts WHERE visibility_radius = $1 AND published_at IS NOT NULL AND url_id IS NULL AND subject NOT LIKE 'Paging: %'`, v), v)
	}

	assert.Equal(t, 1, count(t, db, `SELECT count(*) FROM posts WHERE published_at IS NULL`), "draft")
	assert.Equal(t, 1, count(t, db, `SELECT count(*) FROM posts WHERE url_id IS NOT NULL`), "url post")
	assert.Equal(t, 1, count(t, db, `SELECT count(*) FROM post_shares`))
	assert.Equal(t, 1, count(t, db, `SELECT count(*) FROM post_prompts WHERE dismissed_at IS NULL AND post_id IS NULL`), "open prompt")

	// Comment thread three levels deep.
	assert.Equal(t, 3, count(t, db, `SELECT count(*) FROM post_comments c JOIN posts p ON p.id = c.post_id WHERE p.subject NOT LIKE 'Paging: %'`))
	assert.Equal(t, 1, count(t, db, `SELECT count(*) FROM post_comments c3
		JOIN post_comments c2 ON c3.parent_comment_id = c2.id
		JOIN post_comments c1 ON c2.parent_comment_id = c1.id
		WHERE c1.parent_comment_id IS NULL AND c3.top_comment_id = c1.id`))

	keys, err := factory.ListAPIKeys(ctx, db, alice)
	require.NoError(t, err)
	require.Len(t, keys, 1)
	assert.Equal(t, AliceAPIKey, keys[0].APIKey)

	assert.Equal(t, 1, count(t, db, `SELECT count(*) FROM rss_feeds WHERE url = $1`, FeedURL))
	assert.Equal(t, 3, count(t, db, `SELECT count(*) FROM rss_items i JOIN rss_feeds f ON f.id = i.feed_id WHERE f.url = $1`, FeedURL))

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
	assert.Equal(t, 3, count(t, db, `SELECT count(*) FROM post_comments c JOIN posts p ON p.id = c.post_id WHERE p.subject NOT LIKE 'Paging: %'`))
}

func TestSeed_RefusesOnFly(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	o := options(true)
	o.Production = true

	var out bytes.Buffer

	require.ErrorContains(t, Run(context.Background(), db.DB, &out, o), "FLY_APP_NAME")
	assert.Empty(t, out.String())
	assert.Equal(t, 0, count(t, db, `SELECT count(*) FROM users`))
}

// The paging data puts every paged list past its first page at the default
// page size, Alice's feed with all three kinds on it, and Carol's RSS feed
// at its cap.
func TestSeed_FillsEveryPagedList(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := testdb.New(t).DB

	require.NoError(t, Run(ctx, db.DB, &bytes.Buffer{}, options(false)))

	svc := reading.New(repo.New(db))
	alice := testutil.Must(factory.GetUser(ctx, db, userID(t, db, "alice")))(t)

	feed := testutil.Must(svc.Feed(ctx, alice, ""))(t)
	require.Len(t, feed.Items, reading.DefaultPageSize)
	require.NotEmpty(t, feed.Next, "alice's feed")

	// every seeded item reaches the feed, and the kinds interleave from the
	// first page on
	kinds := map[string]int{}
	firstPageKinds := map[string]bool{}
	for page, n := feed, 0; ; n++ {
		for _, item := range page.Items {
			kind := "post"
			switch {
			case item.Comment != nil:
				kind = "comment"
				if strings.HasPrefix(item.Comment.Post.PostSubject(), PagingPrefix) {
					kinds["paging comment"]++
				}
			case item.FeedItem != nil:
				kind = "rss"
			}

			kinds[kind]++
			if n == 0 {
				firstPageKinds[kind] = true
			}
		}

		if page.Next == "" {
			break
		}

		page = testutil.Must(svc.Feed(ctx, alice, page.Next))(t)
	}
	require.GreaterOrEqual(t, kinds["post"], PagingPosts)
	require.Equal(t, PagingComments, kinds["paging comment"])
	require.Equal(t, PagingFeedItems, kinds["rss"])
	require.Len(t, firstPageKinds, 3, "all three kinds on the first page")

	explore := testutil.Must(svc.Explore(ctx, alice, ""))(t)
	require.NotEmpty(t, explore.Next, "explore")

	index := testutil.Must(svc.PublicPosts(ctx, ""))(t)
	require.NotEmpty(t, index.Next, "the index")

	journal := testutil.Must(svc.Journal(ctx, nil, "carol", ""))(t)
	require.NotEmpty(t, journal.Next, "carol's journal")

	rss := testutil.Must(svc.PublicFeed(ctx, "carol"))(t)
	require.Len(t, rss.Posts, reading.DefaultRSSLimit, "carol's RSS feed")
}
