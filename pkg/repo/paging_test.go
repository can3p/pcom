package repo_test

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
)

type pagedItem struct {
	at time.Time
	id string
}

// pagedSource is one list the feeds page through: add inserts one item
// sorted at that time, list fetches a page.
type pagedSource struct {
	kind string
	add  func(at time.Time) string
	list func(page repo.Page) ([]pagedItem, error)
}

func publishedAt(at time.Time) factory.PostOpt {
	return func(p *model.Post) { p.PublishedAt = new(at) }
}

func posts(ps []*model.Post, err error) ([]pagedItem, error) {
	return lo.Map(ps, func(p *model.Post, _ int) pagedItem { return pagedItem{*p.PublishedAt, p.ID} }), err
}

// pagedSources builds each source on its own users. Only the
// PublishedPostsByProfile one writes public posts, as that query lists every
// author's.
func pagedSources(t *testing.T, ctx context.Context, db repo.Executor) map[string]pagedSource {
	store := repo.Using(db)
	addPost := func(vis model.PostVisibility) (string, func(time.Time) string) {
		author := testutil.Must(factory.User(ctx, db, factory.WithVisibility(model.ProfileVisibilityPublic)))(t)
		return author.ID, func(at time.Time) string {
			return testutil.Must(factory.Post(ctx, db, author.ID, publishedAt(at), factory.Visibility(vis)))(t).ID
		}
	}

	ofID, ofAdd := addPost(model.PostVisibilityDirectOnly)
	usersID, usersAdd := addPost(model.PostVisibilityDirectOnly)
	_, profileAdd := addPost(model.PostVisibilityPublic)

	reader := testutil.Must(factory.User(ctx, db))(t)
	feed := testutil.Must(factory.RSSFeed(ctx, db))(t)

	commenter := testutil.Must(factory.User(ctx, db))(t)
	commented := testutil.Must(factory.Post(ctx, db, reader.ID, factory.Published()))(t)

	return map[string]pagedSource{
		"PublishedPostsOf": {repo.KindPost, ofAdd, func(p repo.Page) ([]pagedItem, error) {
			return posts(store.PublishedPostsOf(ctx, ofID, nil, false, p))
		}},
		"PublishedPostsOfUsers": {repo.KindPost, usersAdd, func(p repo.Page) ([]pagedItem, error) {
			return posts(store.PublishedPostsOfUsers(ctx, []string{usersID}, nil, nil, p))
		}},
		"PublishedPostsByProfile": {repo.KindPost, profileAdd, func(p repo.Page) ([]pagedItem, error) {
			return posts(store.PublishedPostsByProfile(ctx, model.PostVisibilityPublic,
				[]model.ProfileVisibility{model.ProfileVisibilityPublic}, p))
		}},
		"UndismissedFeedItems": {repo.KindRSSItem, func(at time.Time) string {
			item := testutil.Must(factory.RSSItem(ctx, db, feed.ID))(t)
			return testutil.Must(factory.UserFeedItem(ctx, db, reader.ID, item.ID,
				func(i *model.UserFeedItem) { i.CreatedAt = at }))(t).ID
		}, func(p repo.Page) ([]pagedItem, error) {
			items, err := store.UndismissedFeedItems(ctx, reader.ID, p)
			return lo.Map(items, func(i *model.UserFeedItem, _ int) pagedItem { return pagedItem{i.CreatedAt, i.ID} }), err
		}},
		"CommentsOnPostsNotBy": {repo.KindComment, func(at time.Time) string {
			return testutil.Must(factory.Comment(ctx, db, commented.ID, commenter.ID,
				func(c *model.PostComment) { c.CreatedAt = at }))(t).ID
		}, func(p repo.Page) ([]pagedItem, error) {
			comments, err := store.CommentsOnPostsNotBy(ctx, []string{commented.ID}, reader.ID, p)
			return lo.Map(comments, func(c *model.PostComment, _ int) pagedItem { return pagedItem{c.CreatedAt, c.ID} }), err
		}},
	}
}

// Every source pages newest first, ties broken by ID, so that walking the
// pages from the cursor of each last item visits every item exactly once,
// even when items sharing a timestamp straddle a page boundary. A page whose
// cursor is another kind at the same time keeps or skips the whole tie by
// the kinds' order.
func TestPaging(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	base := time.Now().UTC().Truncate(time.Microsecond)
	tie := base.Add(-time.Microsecond)

	for name, src := range pagedSources(t, ctx, db) {
		t.Run(name, func(t *testing.T) {
			// oldest first, the reverse of the expected order
			var want []pagedItem
			for _, at := range []time.Time{base.Add(-2 * time.Microsecond), tie, tie, tie, base} {
				want = append(want, pagedItem{at, src.add(at)})
			}
			slices.SortFunc(want, func(a, b pagedItem) int {
				if c := b.at.Compare(a.at); c != 0 {
					return c
				}
				return strings.Compare(b.id, a.id)
			})
			wantIDs := lo.Map(want, func(i pagedItem, _ int) string { return i.id })

			var got []string
			page := repo.Page{Limit: 2}
			for range 4 {
				items, err := src.list(page)
				require.NoError(t, err)
				got = append(got, lo.Map(items, func(i pagedItem, _ int) string { return i.id })...)
				if len(items) < page.Limit {
					break
				}
				last := items[len(items)-1]
				page = repo.Page{Before: last.at, BeforeKind: src.kind, BeforeID: last.id, Limit: 2}
			}
			require.Equal(t, wantIDs, got)

			all, err := src.list(repo.Page{})
			require.NoError(t, err)
			require.Equal(t, wantIDs, lo.Map(all, func(i pagedItem, _ int) string { return i.id }), "no limit")

			after := func(kind string) []string {
				items, err := src.list(repo.Page{Before: tie, BeforeKind: kind, BeforeID: want[2].id})
				require.NoError(t, err)
				return lo.Map(items, func(i pagedItem, _ int) string { return i.id })
			}
			for _, kind := range []string{repo.KindComment, repo.KindPost, repo.KindRSSItem} {
				switch {
				case kind < src.kind:
					require.Equal(t, wantIDs[4:], after(kind), "an earlier kind at the tie skips it")
				case kind > src.kind:
					require.Equal(t, wantIDs[1:], after(kind), "a later kind at the tie keeps it")
				default:
					require.Equal(t, wantIDs[3:], after(kind), "the same kind continues by ID")
				}
			}
		})
	}
}
