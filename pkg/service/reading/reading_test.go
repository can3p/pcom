package reading

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/postops"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/google/uuid"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
	"github.com/volatiletech/null/v8"
)

// The private RSS feed is read without a session: the token stands for its
// owner, and it lists exactly the posts the owner reads in their feed. A
// direct connection's posts are all there; a second-degree author's only
// when shared that far.
func TestPrivateFeed(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	svc := New(repo.Using(db))

	reader := testutil.Must(factory.User(ctx, db))(t)
	direct := testutil.Must(factory.User(ctx, db))(t)
	second := testutil.Must(factory.User(ctx, db))(t)
	connect(t, db, ctx, reader.ID, direct.ID)
	connect(t, db, ctx, direct.ID, second.ID)

	post := func(authorID string, vis core.PostVisibility) string {
		return testutil.Must(factory.Post(ctx, db, authorID, factory.Published(), factory.Visibility(vis)))(t).ID
	}

	want := []string{
		post(direct.ID, core.PostVisibilityDirectOnly),
		post(direct.ID, core.PostVisibilitySecondDegree),
		post(direct.ID, core.PostVisibilityPublic),
		post(second.ID, core.PostVisibilitySecondDegree),
		post(second.ID, core.PostVisibilityPublic),
	}
	post(second.ID, core.PostVisibilityDirectOnly)
	testutil.Must(factory.Post(ctx, db, direct.ID, factory.Visibility(core.PostVisibilityPublic)))(t) // a draft

	token := testutil.Must(repo.RegenerateFeedToken(ctx, db, reader.ID))(t)

	feed, err := svc.PrivateFeed(ctx, token.Token)
	require.NoError(t, err)
	require.Equal(t, reader.ID, feed.Owner.ID)
	require.ElementsMatch(t, want, lo.Map(feed.Posts, func(p *postops.Post, _ int) string { return p.ID }))

	_, err = svc.PrivateFeed(ctx, uuid.NewString())
	require.ErrorIs(t, err, service.ErrNotFound, "an unknown token")

	_, err = svc.Feed(ctx, nil, "")
	require.ErrorIs(t, err, service.ErrNeedsLogin, "the feed without a login")
}

// The public feed (Q15) lists a post only when it is published, public, and
// its author's profile is public; newest publication first.
// Nobody reads it as an actor, so no post carries comments or actions.
func TestPublicPosts(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	svc := New(repo.Using(db))

	type row struct {
		name string
		id   string
		want bool
	}

	var rows []row
	var public *core.User
	for _, profile := range core.AllProfileVisibility() {
		author := testutil.Must(factory.User(ctx, db, factory.WithVisibility(profile)))(t)
		if profile == core.ProfileVisibilityPublic {
			public = author
		}
		for _, vis := range core.AllPostVisibility() {
			for _, published := range []bool{true, false} {
				opts := []factory.PostOpt{factory.Visibility(vis)}
				if published {
					opts = append(opts, factory.Published())
				}
				rows = append(rows, row{
					name: fmt.Sprintf("%s profile, %s post, published %v", profile, vis, published),
					id:   testutil.Must(factory.Post(ctx, db, author.ID, opts...))(t).ID,
					want: published && vis == core.PostVisibilityPublic && profile == core.ProfileVisibilityPublic,
				})
			}
		}
	}

	newer := testutil.Must(factory.Post(ctx, db, public.ID, factory.Published(), factory.Visibility(core.PostVisibilityPublic)))(t).ID
	newest := testutil.Must(factory.Post(ctx, db, public.ID, factory.Published(), factory.Visibility(core.PostVisibilityPublic)))(t).ID

	page, err := svc.PublicPosts(ctx, "")
	require.NoError(t, err)
	for _, p := range page.Posts {
		require.Equal(t, public.ID, p.Author.ID)
		require.Equal(t, &postops.PostCapabilities{}, p.Capabilities)
		require.Empty(t, p.Comments)
		require.Zero(t, p.CommentsNumber)
	}

	all := lo.Map(page.Posts, func(p *postops.Post, _ int) string { return p.ID })
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			require.Equal(t, r.want, lo.Contains(all, r.id))
		})
	}

	oldest, _ := lo.Find(rows, func(r row) bool { return r.want })
	require.Equal(t, []string{newest, newer, oldest.id}, all, "newest publication first")
}

// Explore, the public posts and a journal (anonymous, and a direct
// connection's, which loads stats) page by publication time: the pages
// together list every post once, newest first then by ID, across a tie at the
// boundary, and the last page has no Next, also when it is exactly full.
// Explore adds the profiles open to registered users. The RSS outputs aren't
// paged: they stop at RSSLimit. A cursor that doesn't parse is invalid input.
func TestPostPages(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name               string
		public, registered int
	}{
		{"two pages", RSSLimit + 1, 4},
		{"exactly full", PageSize, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Explore and the public posts list every author's posts: a
			// database of its own
			db := testdb.New(t).DB
			ctx := context.Background()
			svc := New(repo.Using(db))

			author := testutil.Must(factory.User(ctx, db, factory.WithVisibility(core.ProfileVisibilityPublic)))(t)
			registered := testutil.Must(factory.User(ctx, db, factory.WithVisibility(core.ProfileVisibilityRegisteredUsers)))(t)
			reader := testutil.Must(factory.User(ctx, db))(t)
			connect(t, db, ctx, reader.ID, author.ID)

			base := time.Now().UTC().Truncate(time.Microsecond)
			type entry struct {
				at time.Time
				id string
			}
			add := func(authorID string, ago int) entry {
				at := base.Add(-time.Duration(ago) * time.Microsecond)
				post := testutil.Must(factory.Post(ctx, db, authorID, factory.Visibility(core.PostVisibilityPublic),
					func(p *core.Post) { p.PublishedAt = null.TimeFrom(at) }))(t)
				return entry{at, post.ID}
			}
			var public, all []entry
			for i := tc.public - 1; i >= 0; i-- { // oldest first; posts 28 to 32 share a time
				ago := i
				if i > 32 {
					ago = i - 4
				} else if i >= 28 {
					ago = 28
				}
				public = append(public, add(author.ID, ago))
			}
			all = slices.Clone(public)
			for i := range tc.registered {
				all = append(all, add(registered.ID, 30-i*10))
			}
			order := func(entries []entry) []string {
				slices.SortFunc(entries, func(a, b entry) int {
					if c := b.at.Compare(a.at); c != 0 {
						return c
					}
					return strings.Compare(b.id, a.id)
				})
				return lo.Map(entries, func(e entry, _ int) string { return e.id })
			}
			wantPublic, wantAll := order(public), order(all)

			ids := func(posts []*postops.Post) []string {
				return lo.Map(posts, func(p *postops.Post, _ int) string { return p.ID })
			}
			journal := func(actor *core.User) func(string) ([]*postops.Post, string, error) {
				return func(c string) ([]*postops.Post, string, error) {
					j, err := svc.Journal(ctx, actor, author.Username, c)
					if err != nil {
						return nil, "", err
					}
					return j.Posts, j.Next, nil
				}
			}
			posts := func(list func(string) (*Posts, error)) func(string) ([]*postops.Post, string, error) {
				return func(c string) ([]*postops.Post, string, error) {
					page, err := list(c)
					if err != nil {
						return nil, "", err
					}
					return page.Posts, page.Next, nil
				}
			}
			for name, l := range map[string]struct {
				list func(cursor string) ([]*postops.Post, string, error)
				want []string
			}{
				"explore":            {posts(func(c string) (*Posts, error) { return svc.Explore(ctx, reader, c) }), wantAll},
				"public posts":       {posts(func(c string) (*Posts, error) { return svc.PublicPosts(ctx, c) }), wantPublic},
				"anonymous journal":  {journal(nil), wantPublic},
				"connection journal": {journal(reader), wantPublic},
			} {
				t.Run(name, func(t *testing.T) {
					first, next, err := l.list("")
					require.NoError(t, err)
					require.Equal(t, l.want[:PageSize], ids(first))
					if len(l.want) == PageSize {
						require.Empty(t, next)
						return
					}

					second, last, err := l.list(next)
					require.NoError(t, err)
					require.Len(t, second, len(l.want)-PageSize)
					require.Equal(t, l.want, append(ids(first), ids(second)...))
					require.Empty(t, last)

					_, _, err = l.list("bm90IGEgY3Vyc29y")
					var invalid *service.ValidationError
					require.ErrorAs(t, err, &invalid)
				})
			}

			if tc.public <= RSSLimit {
				return
			}

			token := testutil.Must(repo.RegenerateFeedToken(ctx, db, reader.ID))(t)
			private, err := svc.PrivateFeed(ctx, token.Token)
			require.NoError(t, err)
			require.Equal(t, wantPublic[:RSSLimit], ids(private.Posts), "the private feed")

			feed, err := svc.PublicFeed(ctx, author.Username)
			require.NoError(t, err)
			require.Equal(t, wantPublic[:RSSLimit], ids(feed.Posts), "the author's public feed")

			rss, err := svc.PublicPostsRSS(ctx)
			require.NoError(t, err)
			require.Equal(t, wantPublic[:RSSLimit], ids(rss), "the public feed")
		})
	}
}

// The journal carries the author's About text for whoever may open it, and a
// journal that is hidden from the visitor is not found, text and all.
func TestJournalAbout(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	svc := New(repo.Using(db))

	public := testutil.Must(factory.User(ctx, db, factory.WithVisibility(core.ProfileVisibilityPublic), factory.WithProfileAbout("About **me**")))(t)
	hidden := testutil.Must(factory.User(ctx, db, factory.WithVisibility(core.ProfileVisibilityConnections), factory.WithProfileAbout("secret")))(t)
	none := testutil.Must(factory.User(ctx, db, factory.WithVisibility(core.ProfileVisibilityPublic)))(t)
	visitor := testutil.Must(factory.User(ctx, db))(t)
	connect(t, db, ctx, visitor.ID, hidden.ID)

	j, err := svc.Journal(ctx, nil, public.Username)
	require.NoError(t, err)
	require.Equal(t, "About **me**", j.About, "an anonymous visitor of a public journal")

	j, err = svc.Journal(ctx, nil, none.Username)
	require.NoError(t, err)
	require.Empty(t, j.About)

	_, err = svc.Journal(ctx, nil, hidden.Username)
	require.ErrorIs(t, err, service.ErrNotFound)

	_, err = svc.Journal(ctx, testutil.Must(factory.User(ctx, db))(t), hidden.Username)
	require.ErrorIs(t, err, service.ErrNotFound, "a stranger")

	j, err = svc.Journal(ctx, visitor, hidden.Username)
	require.NoError(t, err)
	require.Equal(t, "secret", j.About, "a connection of a connections-only journal")
}
