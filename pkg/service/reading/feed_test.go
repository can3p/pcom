package reading

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/google/uuid"
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

type feedEntry struct {
	at       time.Time
	kind, id string
}

// byFeedOrder sorts entries the way the feed promises: newest first, then
// kind (rss, post, comment), then ID, descending.
func byFeedOrder(a, b feedEntry) int {
	if c := b.at.Compare(a.at); c != 0 {
		return c
	}
	if c := strings.Compare(b.kind, a.kind); c != 0 {
		return c
	}
	return strings.Compare(b.id, a.id)
}

func feedIDs(items []*FeedItem) []string {
	return lo.Map(items, func(i *FeedItem, _ int) string {
		switch {
		case i.Post != nil:
			return i.Post.ID
		case i.FeedItem != nil:
			return i.FeedItem.ID
		default:
			return i.Comment.ID
		}
	})
}

// The feed merges posts, RSS items and comments newest first, ties broken by
// kind and ID, and pages through them without gaps or repeats. In "two
// pages" the posts outnumber a page and the cut falls inside a tie of posts
// that RSS items and comments share; an exactly full page has no Next. Only
// the first page carries the connections, prompts and feed token.
func TestFeedPages(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	svc := New(repo.Using(db))
	base := time.Now().UTC().Truncate(time.Microsecond)
	us := func(n int) time.Time { return base.Add(-time.Duration(n) * time.Microsecond) }
	n := func(kind string, count int, at func(i int) time.Time) []feedEntry {
		return lo.Times(count, func(i int) feedEntry { return feedEntry{at: at(i), kind: kind} })
	}

	twoPages := slices.Concat(
		n(repo.KindPost, 22, us),
		n(repo.KindPost, 14, func(int) time.Time { return us(22) }),
		n(repo.KindRSSItem, 5, func(i int) time.Time { return us([]int{3, 9, 15, 22, 23}[i]) }),
		n(repo.KindComment, 4, func(i int) time.Time { return us([]int{6, 12, 22, 23}[i]) }),
	)
	exactlyFull := slices.Concat(
		n(repo.KindPost, 20, us),
		n(repo.KindRSSItem, 5, func(i int) time.Time { return us(i * 4) }),
		n(repo.KindComment, 5, func(i int) time.Time { return us(i*4 + 1) }),
	)

	for name, entries := range map[string][]feedEntry{"two pages": twoPages, "exactly full": exactlyFull} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			reader := testutil.Must(factory.User(ctx, db))(t)
			direct := testutil.Must(factory.User(ctx, db))(t)
			connect(t, db, ctx, reader.ID, direct.ID)
			ownPost := testutil.Must(factory.Post(ctx, db, reader.ID))(t)
			feed := testutil.Must(factory.RSSFeed(ctx, db))(t)
			testutil.Must(factory.PostPrompt(ctx, db, direct.ID, reader.ID))(t)
			testutil.Must(repo.RegenerateFeedToken(ctx, db, reader.ID))(t)

			// oldest first, the reverse of the feed's order
			slices.SortFunc(entries, func(a, b feedEntry) int { return a.at.Compare(b.at) })
			for i, e := range entries {
				switch e.kind {
				case repo.KindPost:
					entries[i].id = testutil.Must(factory.Post(ctx, db, direct.ID,
						func(p *core.Post) { p.PublishedAt = null.TimeFrom(e.at) }))(t).ID
				case repo.KindRSSItem:
					item := testutil.Must(factory.RSSItem(ctx, db, feed.ID))(t)
					entries[i].id = testutil.Must(factory.UserFeedItem(ctx, db, reader.ID, item.ID,
						func(i *core.UserFeedItem) { i.CreatedAt = e.at }))(t).ID
				default:
					entries[i].id = testutil.Must(factory.Comment(ctx, db, ownPost.ID, direct.ID,
						func(c *core.PostComment) { c.CreatedAt = e.at }))(t).ID
				}
			}
			slices.SortFunc(entries, byFeedOrder)
			want := lo.Map(entries, func(e feedEntry, _ int) string { return e.id })

			first, err := svc.Feed(ctx, reader, "")
			require.NoError(t, err)
			require.Equal(t, want[:PageSize], feedIDs(first.Items))
			require.Len(t, first.DirectConnections, 1)
			require.Len(t, first.OpenPrompts, 1)
			require.NotNil(t, first.FeedToken)

			if len(want) == PageSize {
				require.Empty(t, first.Next)
				return
			}

			second, err := svc.Feed(ctx, reader, first.Next)
			require.NoError(t, err)
			require.Equal(t, want[PageSize:], feedIDs(second.Items))
			require.Empty(t, second.Next)
			require.Nil(t, second.DirectConnections)
			require.Empty(t, second.OpenPrompts)
			require.Nil(t, second.FeedToken)

			_, err = svc.Feed(ctx, reader, "not a cursor")
			var invalid *service.ValidationError
			require.ErrorAs(t, err, &invalid)
		})
	}
}

func TestParseCursor(t *testing.T) {
	t.Parallel()

	id := uuid.NewString()
	at := time.Now().UTC().Truncate(time.Microsecond)
	got, err := ParseCursor(Cursor{Time: at, Kind: repo.KindRSSItem, ID: id}.String())
	require.NoError(t, err)
	require.True(t, at.Equal(got.Time), "microseconds survive")
	require.Equal(t, repo.KindRSSItem, got.Kind)
	require.Equal(t, id, got.ID)

	got, err = ParseCursor("")
	require.NoError(t, err)
	require.Zero(t, got, "the first page")

	b64 := func(s string) string { return base64.RawURLEncoding.EncodeToString([]byte(s)) }
	for name, cursor := range map[string]string{
		"not base64":         "!!!",
		"wrong part count":   b64("1|post"),
		"zero micros":        b64("0|post|" + id),
		"non-numeric micros": b64("x|post|" + id),
		"unknown kind":       b64("1|draft|" + id),
		"non-UUID id":        b64("1|post|42"),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := ParseCursor(cursor)
			var invalid *service.ValidationError
			require.ErrorAs(t, err, &invalid)
		})
	}
}
