package reading

import (
	"context"
	"fmt"
	"slices"
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

// Explore, the public posts and a journal page by publication time: two
// pages visit every post once, newest first, across a tie at the boundary,
// and the last page has no Next. The RSS outputs aren't paged: they stop at
// RSSLimit. A cursor that doesn't parse is invalid input.
func TestPostPages(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	svc := New(repo.Using(db))

	author := testutil.Must(factory.User(ctx, db, factory.WithVisibility(core.ProfileVisibilityPublic)))(t)
	reader := testutil.Must(factory.User(ctx, db))(t)
	connect(t, db, ctx, reader.ID, author.ID)

	base := time.Now().UTC().Truncate(time.Second)
	var want []string
	for i := RSSLimit; i >= 0; i-- { // oldest first; posts 28 to 32 share a time
		at := base.Add(-time.Duration((i+2)/5) * time.Minute)
		post := testutil.Must(factory.Post(ctx, db, author.ID, factory.Visibility(core.PostVisibilityPublic),
			func(p *core.Post) { p.PublishedAt = null.TimeFrom(at) }))(t)
		want = append(want, post.ID)
	}
	slices.Reverse(want)

	ids := func(posts []*postops.Post) []string {
		return lo.Map(posts, func(p *postops.Post, _ int) string { return p.ID })
	}
	lists := map[string]func(cursor string) ([]*postops.Post, string, error){
		"explore": func(c string) ([]*postops.Post, string, error) {
			page, err := svc.Explore(ctx, reader, c)
			if err != nil {
				return nil, "", err
			}
			return page.Posts, page.Next, nil
		},
		"public posts": func(c string) ([]*postops.Post, string, error) {
			page, err := svc.PublicPosts(ctx, c)
			if err != nil {
				return nil, "", err
			}
			return page.Posts, page.Next, nil
		},
		"journal": func(c string) ([]*postops.Post, string, error) {
			journal, err := svc.Journal(ctx, nil, author.Username, c)
			if err != nil {
				return nil, "", err
			}
			return journal.Posts, journal.Next, nil
		},
	}
	for name, list := range lists {
		t.Run(name, func(t *testing.T) {
			first, next, err := list("")
			require.NoError(t, err)
			require.Len(t, first, PageSize)
			second, last, err := list(next)
			require.NoError(t, err)
			require.Empty(t, last)
			ordered := slices.IsSortedFunc(append(first, second...), func(a, b *postops.Post) int {
				return b.PublishedAt.Time.Compare(a.PublishedAt.Time)
			})
			require.True(t, ordered, "newest first")
			require.ElementsMatch(t, want, append(ids(first), ids(second)...))

			_, _, err = list("bm90IGEgY3Vyc29y")
			var invalid *service.ValidationError
			require.ErrorAs(t, err, &invalid)
		})
	}

	token := testutil.Must(repo.RegenerateFeedToken(ctx, db, reader.ID))(t)
	private, err := svc.PrivateFeed(ctx, token.Token)
	require.NoError(t, err)
	require.ElementsMatch(t, want[:RSSLimit], ids(private.Posts), "the private feed")

	public, err := svc.PublicFeed(ctx, author.Username)
	require.NoError(t, err)
	require.ElementsMatch(t, want[:RSSLimit], ids(public.Posts), "the author's public feed")

	all, err := svc.PublicPostsRSS(ctx)
	require.NoError(t, err)
	require.Len(t, all, RSSLimit, "the public feed")
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
