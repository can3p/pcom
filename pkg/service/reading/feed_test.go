package reading

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/volatiletech/null/v8"
	"github.com/volatiletech/sqlboiler/v4/boil"
)

// errFeedInjectedQuery is the error a failingExecutor reports once its query
// budget runs out.
var errFeedInjectedQuery = errors.New("feed: injected query failure")

// failingExecutor wraps a boil.ContextExecutor and lets exactly failAfter
// queries through before failing every one after that, to reach the error
// branches a real database only takes on an actual failure.
type failingExecutor struct {
	boil.ContextExecutor
	calls     int
	failAfter int
}

func (e *failingExecutor) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	e.calls++
	if e.calls > e.failAfter {
		return nil, errFeedInjectedQuery
	}
	return e.ContextExecutor.QueryContext(ctx, query, args...)
}

func (e *failingExecutor) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	e.calls++
	if e.calls > e.failAfter {
		// *sql.Row can't carry an injected error, so run a query that fails
		return e.ContextExecutor.QueryRowContext(ctx, "select 1/0")
	}
	return e.ContextExecutor.QueryRowContext(ctx, query, args...)
}

// connect creates a direct connection between a and b, or fails the test.
func connect(t *testing.T, db boil.ContextExecutor, ctx context.Context, aID, bID string) {
	t.Helper()
	_, _, err := factory.Connect(ctx, db, aID, bID)
	require.NoError(t, err)
}

func TestGetComments(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	user := testutil.Must(factory.User(ctx, db))(t)
	direct := testutil.Must(factory.User(ctx, db))(t)
	connect(t, db, ctx, user.ID, direct.ID)

	// a comment by someone else on the user's own post: always included.
	ownPost := testutil.Must(factory.Post(ctx, db, user.ID))(t)
	ownPostComment := testutil.Must(factory.Comment(ctx, db, ownPost.ID, direct.ID))(t)

	// a direct connection's post the user has participated in: a further
	// comment from someone else on it is included too.
	participatedPost := testutil.Must(factory.Post(ctx, db, direct.ID))(t)
	testutil.Must(factory.Comment(ctx, db, participatedPost.ID, user.ID))(t)

	otherCommenter := testutil.Must(factory.User(ctx, db))(t)
	connect(t, db, ctx, user.ID, otherCommenter.ID)
	participatedComment := testutil.Must(factory.Comment(ctx, db, participatedPost.ID, otherCommenter.ID))(t)

	// a direct connection's post the user never commented on: excluded,
	// even though someone else left a comment on it.
	untouchedPost := testutil.Must(factory.Post(ctx, db, direct.ID))(t)
	testutil.Must(factory.Comment(ctx, db, untouchedPost.ID, otherCommenter.ID))(t)

	items, err := New(repo.Using(db)).feedComments(ctx, user.ID, repo.Page{})
	require.NoError(t, err)

	gotIDs := make([]string, len(items))
	for i, item := range items {
		require.NotNil(t, item.Comment)
		gotIDs[i] = item.Comment.ID
	}

	assert.ElementsMatch(t, []string{ownPostComment.ID, participatedComment.ID}, gotIDs)

	for _, item := range items {
		require.NotEqual(t, user.ID, item.Comment.UserID, "the user's own comments are never echoed back")
	}
}

func TestGetComments_QueryErrorsPropagate(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()

	user := testutil.Must(factory.User(ctx, db))(t)

	for _, tc := range []struct {
		name      string
		failAfter int
	}{
		{"own comments lookup fails", 0},
		{"direct user ids lookup fails", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			exec := &failingExecutor{ContextExecutor: db, failAfter: tc.failAfter}
			_, err := New(repo.Using(exec)).feedComments(ctx, user.ID, repo.Page{})
			require.ErrorIs(t, err, errFeedInjectedQuery)
		})
	}
}

// The feed merges posts, RSS items and comments newest first, ties broken by
// kind and ID: two pages visit every item once, in that total order, with
// items sharing a timestamp across sources and across the page boundary.
// Only the first page carries the connections, and the last has no Next.
func TestFeedPages(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	svc := New(repo.Using(db))

	reader := testutil.Must(factory.User(ctx, db))(t)
	direct := testutil.Must(factory.User(ctx, db))(t)
	connect(t, db, ctx, reader.ID, direct.ID)
	ownPost := testutil.Must(factory.Post(ctx, db, reader.ID))(t)
	feed := testutil.Must(factory.RSSFeed(ctx, db))(t)

	base := time.Now().UTC().Truncate(time.Second)
	const perSource = 12
	var want []string
	// oldest first, the reverse of the feed's order; 4 timestamps shared by 9
	// items each, so the boundary after item 30 falls inside a tie
	for i := perSource - 1; i >= 0; i-- {
		at := base.Add(-time.Duration(i%4) * time.Hour)
		post := testutil.Must(factory.Post(ctx, db, direct.ID, func(p *core.Post) { p.PublishedAt = null.TimeFrom(at) }))(t)
		item := testutil.Must(factory.RSSItem(ctx, db, feed.ID))(t)
		feedItem := testutil.Must(factory.UserFeedItem(ctx, db, reader.ID, item.ID, func(i *core.UserFeedItem) { i.CreatedAt = at }))(t)
		comment := testutil.Must(factory.Comment(ctx, db, ownPost.ID, direct.ID, func(c *core.PostComment) { c.CreatedAt = at }))(t)
		want = append(want, post.ID, feedItem.ID, comment.ID)
	}

	first, err := svc.Feed(ctx, reader, "")
	require.NoError(t, err)
	require.Len(t, first.Items, PageSize)
	require.NotEmpty(t, first.Next)
	require.Len(t, first.DirectConnections, 1)

	second, err := svc.Feed(ctx, reader, first.Next)
	require.NoError(t, err)
	require.Len(t, second.Items, 3*perSource-PageSize)
	require.Empty(t, second.Next)
	require.Nil(t, second.DirectConnections)

	items := append(first.Items, second.Items...)
	require.True(t, slices.IsSortedFunc(items, compareItems), "a total order, newest first")
	require.ElementsMatch(t, want, lo.Map(items, func(i *FeedItem, _ int) string { return cursorOf(i).ID }))

	_, err = svc.Feed(ctx, reader, "not a cursor")
	var invalid *service.ValidationError
	require.ErrorAs(t, err, &invalid)
}
