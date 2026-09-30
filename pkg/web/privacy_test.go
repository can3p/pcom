package web_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service/reading"
	"github.com/can3p/pcom/pkg/service/shares"
	"github.com/samber/mo"

	"github.com/can3p/pcom/pkg/auth"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/postops"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/ginctx"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/can3p/pcom/pkg/util/ginhelpers"
	"github.com/can3p/pcom/pkg/web"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
	"github.com/volatiletech/sqlboiler/v4/boil"
)

// The privacy matrix of pcom, a private social network. It answers, for every
// kind of visitor, every post visibility, every profile visibility and for
// drafts and published posts alike: can the visitor see it, which error do
// they get when they can't, and what can they do with it.
//
// The visitors, relative to the author:
//
//	anon     - not logged in
//	stranger - logged in, no connection path to the author
//	second   - a connection of a direct connection of the author
//	direct   - directly connected to the author
//	author   - the author
type viewer string

const (
	asAnonymous    viewer = "anon"
	asStranger     viewer = "stranger"
	asSecondDegree viewer = "second"
	asDirect       viewer = "direct"
	asAuthor       viewer = "author"
)

var profileVisibilities = []core.ProfileVisibility{
	core.ProfileVisibilityPublic,
	core.ProfileVisibilityRegisteredUsers,
	core.ProfileVisibilityConnections,
}

var postVisibilities = []core.PostVisibility{
	core.PostVisibilityDirectOnly,
	core.PostVisibilitySecondDegree,
	core.PostVisibilityPublic,
}

// access is what a visitor gets when they open a post.
type access string

const (
	// needsLogin: the visitor is sent to log in.
	needsLogin access = "needs login"
	// notFound: the post doesn't exist as far as the visitor can tell.
	notFound access = "not found"
	// reader: the post is shown; comments are hidden and nothing can be done with it.
	reader access = "reader"
	// asCommenter: the post and its comments are shown and the visitor can comment.
	asCommenter access = "commenter"
	// owner: everything, including edit and share.
	owner access = "owner"
)

func (a access) capabilities() postops.PostCapabilities {
	switch a {
	case owner:
		return postops.PostCapabilities{CanViewComments: true, CanLeaveComments: true, CanEdit: true, CanShare: true}
	case asCommenter:
		return postops.PostCapabilities{CanViewComments: true, CanLeaveComments: true}
	default:
		return postops.PostCapabilities{}
	}
}

type postKey struct {
	profile   core.ProfileVisibility
	vis       core.PostVisibility
	published bool
}

// world holds one author per profile visibility, each with one post per
// post visibility, as a draft and published. Every post has a comment and a
// share link. friend is directly connected to every author, fof is connected
// to friend only and stranger to nobody.
type world struct {
	db       *sqlx.DB
	authors  map[core.ProfileVisibility]*core.User
	friend   *core.User
	fof      *core.User
	stranger *core.User
	posts    map[postKey]*core.Post
	shares   map[postKey]*core.PostShare
}

func newWorld(t *testing.T) *world {
	t.Helper()

	db := testdb.New(t).DB
	ctx := context.Background()

	w := &world{
		db:      db,
		authors: map[core.ProfileVisibility]*core.User{},
		posts:   map[postKey]*core.Post{},
		shares:  map[postKey]*core.PostShare{},
	}

	w.friend = testutil.Must(factory.User(ctx, db))(t)
	w.fof = testutil.Must(factory.User(ctx, db))(t)
	w.stranger = testutil.Must(factory.User(ctx, db))(t)

	connect(t, db, ctx, w.friend.ID, w.fof.ID)

	for _, profile := range profileVisibilities {
		author := testutil.Must(factory.User(ctx, db, factory.WithVisibility(profile)))(t)
		w.authors[profile] = author

		connect(t, db, ctx, author.ID, w.friend.ID)

		for _, vis := range postVisibilities {
			for _, published := range []bool{false, true} {
				opts := []factory.PostOpt{factory.Visibility(vis)}
				if published {
					opts = append(opts, factory.Published())
				}

				post := testutil.Must(factory.Post(ctx, db, author.ID, opts...))(t)
				testutil.Must(factory.Comment(ctx, db, post.ID, author.ID))(t)
				share := testutil.Must(factory.PostShare(ctx, db, post.ID))(t)

				key := postKey{profile: profile, vis: vis, published: published}
				w.posts[key] = post
				w.shares[key] = share
			}
		}
	}

	return w
}

// connect creates a direct connection between a and b, or fails the test.
func connect(t *testing.T, db *sqlx.DB, ctx context.Context, aID, bID string) {
	t.Helper()
	_, _, err := factory.Connect(ctx, db, aID, bID)
	require.NoError(t, err)
}

// userData returns the request context and user data of viewer looking at
// the author whose profile visibility is profile.
func (w *world) userData(t *testing.T, viewer viewer, profile core.ProfileVisibility) (*gin.Context, *auth.UserData) {
	t.Helper()

	var user *core.User

	switch viewer {
	case asAnonymous:
	case asStranger:
		user = w.stranger
	case asSecondDegree:
		user = w.fof
	case asDirect:
		user = w.friend
	case asAuthor:
		user = w.authors[profile]
	}

	var opts []ginctx.Option
	if user != nil {
		opts = append(opts, ginctx.WithUser(t, w.db, user.ID))
	}

	c, _ := ginctx.New(t, http.MethodGet, "/", nil, opts...)
	userData := auth.GetUserData(c)

	return c, &userData
}

// postPage, userHome and explore build the pages the way their routes do:
// the reading service decides what the visitor sees, the page builder
// renders it.
func postPage(c *gin.Context, exec boil.ContextExecutor, u *auth.UserData, postID string) mo.Result[*web.SinglePostPage] {
	post, err := reading.New(repo.Using(exec)).Post(c, u.DBUser, postID, false)
	if err != nil {
		return mo.Err[*web.SinglePostPage](err)
	}

	return mo.Ok(web.PostPage(c, u, post))
}

func userHome(c *gin.Context, exec boil.ContextExecutor, u *auth.UserData, username string) mo.Result[*web.UserHomePage] {
	journal, err := reading.New(repo.Using(exec)).Journal(c, u.DBUser, username)
	if err != nil {
		return mo.Err[*web.UserHomePage](err)
	}

	return mo.Ok(web.UserHome(c, u, journal))
}

func explore(c *gin.Context, exec boil.ContextExecutor, u *auth.UserData) mo.Result[*web.FeedPage] {
	posts, err := reading.New(repo.Using(exec)).Explore(c, u.DBUser)
	if err != nil {
		return mo.Err[*web.FeedPage](err)
	}

	return mo.Ok(web.Explore(c, u, posts))
}

func TestPrivacyMatrix(t *testing.T) {
	t.Parallel()

	w := newWorld(t)

	// Opening a post by its URL, /posts/:id. The author's profile visibility
	// doesn't matter here: only the post's own visibility does, so every row
	// holds for all three profile visibilities. A public post of a
	// connections-only profile is readable by anyone with its link, by
	// decision (docs/open-questions.md, Q15), although Explore and the
	// profile page don't list it.
	singlePostSpec := []struct {
		viewer    viewer
		vis       core.PostVisibility
		published bool
		want      access
		bug       string
	}{
		{asAuthor, core.PostVisibilityDirectOnly, true, owner, ""},
		{asAuthor, core.PostVisibilitySecondDegree, true, owner, ""},
		{asAuthor, core.PostVisibilityPublic, true, owner, ""},
		// the author previews their own drafts
		{asAuthor, core.PostVisibilityDirectOnly, false, owner, ""},
		{asAuthor, core.PostVisibilitySecondDegree, false, owner, ""},
		{asAuthor, core.PostVisibilityPublic, false, owner, ""},

		{asDirect, core.PostVisibilityDirectOnly, true, asCommenter, ""},
		{asDirect, core.PostVisibilitySecondDegree, true, asCommenter, ""},
		{asDirect, core.PostVisibilityPublic, true, asCommenter, ""},
		{asDirect, core.PostVisibilityDirectOnly, false, notFound, ""},
		{asDirect, core.PostVisibilitySecondDegree, false, notFound, ""},
		{asDirect, core.PostVisibilityPublic, false, notFound, ""},

		{asSecondDegree, core.PostVisibilityDirectOnly, true, notFound, ""},
		{asSecondDegree, core.PostVisibilitySecondDegree, true, reader, ""},
		{asSecondDegree, core.PostVisibilityPublic, true, reader, ""},
		{asSecondDegree, core.PostVisibilityDirectOnly, false, notFound, ""},
		{asSecondDegree, core.PostVisibilitySecondDegree, false, notFound, ""},
		{asSecondDegree, core.PostVisibilityPublic, false, notFound, ""},

		{asStranger, core.PostVisibilityDirectOnly, true, notFound, ""},
		{asStranger, core.PostVisibilitySecondDegree, true, notFound, ""},
		{asStranger, core.PostVisibilityPublic, true, reader, ""},
		{asStranger, core.PostVisibilityDirectOnly, false, notFound, ""},
		{asStranger, core.PostVisibilitySecondDegree, false, notFound, ""},
		{asStranger, core.PostVisibilityPublic, false, notFound, ""},

		// anonymous visitors are asked to log in rather than told the post is missing
		{asAnonymous, core.PostVisibilityDirectOnly, true, needsLogin, ""},
		{asAnonymous, core.PostVisibilitySecondDegree, true, needsLogin, ""},
		{asAnonymous, core.PostVisibilityPublic, true, reader, ""},
		{asAnonymous, core.PostVisibilityDirectOnly, false, needsLogin, ""},
		{asAnonymous, core.PostVisibilitySecondDegree, false, needsLogin, ""},
		{asAnonymous, core.PostVisibilityPublic, false, needsLogin, ""},
	}

	for _, profile := range profileVisibilities {
		for _, row := range singlePostSpec {
			state := "draft"
			if row.published {
				state = "published"
			}

			name := fmt.Sprintf("SinglePost/%s profile/%s %s post/%s", profile, row.vis, state, row.viewer)

			t.Run(name, func(t *testing.T) {
				t.Parallel()

				if row.bug != "" {
					t.Skip(row.bug)
				}

				key := postKey{profile: profile, vis: row.vis, published: row.published}
				post := w.posts[key]
				c, userData := w.userData(t, row.viewer, profile)

				res := postPage(c, w.db, userData, post.ID)

				switch row.want {
				case needsLogin:
					require.ErrorIs(t, res.Error(), ginhelpers.ErrNeedsLogin)
					return
				case notFound:
					require.ErrorIs(t, res.Error(), ginhelpers.ErrNotFound)
					return
				}

				page, err := res.Get()
				require.NoError(t, err)
				require.Equal(t, post.ID, page.Post.ID)
				require.Equal(t, w.authors[profile].ID, page.Post.Author.ID)
				require.Equal(t, row.want.capabilities(), *page.Post.Capabilities)

				if row.want.capabilities().CanViewComments {
					require.Len(t, page.Comments, 1, "comments are shown")
				} else {
					require.Empty(t, page.Comments, "comments are hidden")
				}

				if row.want.capabilities().CanShare {
					require.NotNil(t, page.PostShare, "the share link is shown")
					require.Equal(t, w.shares[key].ID, page.PostShare.ID)
				} else {
					require.Nil(t, page.PostShare, "the share link is hidden")
				}
			})
		}
	}

	t.Run("SinglePost/unknown post", func(t *testing.T) {
		t.Parallel()

		for _, v := range []viewer{asAnonymous, asStranger} {
			c, userData := w.userData(t, v, core.ProfileVisibilityPublic)
			res := postPage(c, w.db, userData, uuid.NewString())
			require.ErrorIs(t, res.Error(), ginhelpers.ErrNotFound, v)
		}
	})

	// The profile page, /users/:username. Profile visibility decides who may
	// open it at all (a hidden profile is always "not found", so its existence
	// isn't revealed); post visibility then decides which published posts it
	// lists. Drafts are never listed.
	all := postVisibilities
	secondAndPublic := []core.PostVisibility{core.PostVisibilitySecondDegree, core.PostVisibilityPublic}
	publicOnly := []core.PostVisibility{core.PostVisibilityPublic}

	userHomeSpec := []struct {
		profile core.ProfileVisibility
		viewer  viewer
		want    access // notFound, or the access to every listed post
		lists   []core.PostVisibility
		bug     string
	}{
		{core.ProfileVisibilityPublic, asAnonymous, reader, publicOnly, ""},
		{core.ProfileVisibilityPublic, asStranger, reader, publicOnly, ""},
		{core.ProfileVisibilityPublic, asSecondDegree, reader, secondAndPublic, ""},
		{core.ProfileVisibilityPublic, asDirect, asCommenter, all, ""},
		{core.ProfileVisibilityPublic, asAuthor, owner, all, ""},

		{core.ProfileVisibilityRegisteredUsers, asAnonymous, notFound, nil, ""},
		{core.ProfileVisibilityRegisteredUsers, asStranger, reader, publicOnly, ""},
		{core.ProfileVisibilityRegisteredUsers, asSecondDegree, reader, secondAndPublic, ""},
		{core.ProfileVisibilityRegisteredUsers, asDirect, asCommenter, all, ""},
		{core.ProfileVisibilityRegisteredUsers, asAuthor, owner, all, ""},

		{core.ProfileVisibilityConnections, asAnonymous, notFound, nil, ""},
		{core.ProfileVisibilityConnections, asStranger, notFound, nil, ""},
		{core.ProfileVisibilityConnections, asSecondDegree, reader, secondAndPublic, ""},
		{core.ProfileVisibilityConnections, asDirect, asCommenter, all, ""},
		{core.ProfileVisibilityConnections, asAuthor, owner, all, ""},
	}

	for _, row := range userHomeSpec {
		t.Run(fmt.Sprintf("UserHome/%s profile/%s", row.profile, row.viewer), func(t *testing.T) {
			t.Parallel()

			if row.bug != "" {
				t.Skip(row.bug)
			}

			author := w.authors[row.profile]
			c, userData := w.userData(t, row.viewer, row.profile)

			res := userHome(c, w.db, userData, author.Username)

			if row.want == notFound {
				require.ErrorIs(t, res.Error(), ginhelpers.ErrNotFound)
				return
			}

			page, err := res.Get()
			require.NoError(t, err)
			require.Equal(t, author.ID, page.Author.ID)

			wantIDs := []string{}
			for _, vis := range row.lists {
				wantIDs = append(wantIDs, w.posts[postKey{profile: row.profile, vis: vis, published: true}].ID)
			}

			gotIDs := []string{}
			for _, p := range page.Posts {
				gotIDs = append(gotIDs, p.ID)
				require.Equal(t, row.want.capabilities(), *p.Capabilities, "capabilities of %s post", p.VisibilityRadius)
			}

			require.ElementsMatch(t, wantIDs, gotIDs)

			// only a public profile advertises its RSS feed
			if row.profile == core.ProfileVisibilityPublic {
				require.NotEmpty(t, page.RSSFeed)
			} else {
				require.Empty(t, page.RSSFeed)
			}
		})
	}

	t.Run("UserHome/unknown user", func(t *testing.T) {
		t.Parallel()

		c, userData := w.userData(t, asStranger, core.ProfileVisibilityPublic)
		res := userHome(c, w.db, userData, "no-such-user")
		require.ErrorIs(t, res.Error(), ginhelpers.ErrNotFound)
	})

	// Explore, /explore, lists published public posts only, and only of
	// profiles the visitor could open without a connection: public profiles
	// for anonymous visitors, public and registered-users profiles for anyone
	// logged in. Connections-only profiles never show up, whatever the
	// visitor's connections. Nobody gets comments or actions there.
	exploreSpec := []struct {
		viewer   viewer
		profiles []core.ProfileVisibility
	}{
		{asAnonymous, []core.ProfileVisibility{core.ProfileVisibilityPublic}},
		{asStranger, []core.ProfileVisibility{core.ProfileVisibilityPublic, core.ProfileVisibilityRegisteredUsers}},
		{asSecondDegree, []core.ProfileVisibility{core.ProfileVisibilityPublic, core.ProfileVisibilityRegisteredUsers}},
		{asDirect, []core.ProfileVisibility{core.ProfileVisibilityPublic, core.ProfileVisibilityRegisteredUsers}},
	}

	for _, row := range exploreSpec {
		t.Run(fmt.Sprintf("Explore/%s", row.viewer), func(t *testing.T) {
			t.Parallel()

			c, userData := w.userData(t, row.viewer, core.ProfileVisibilityPublic)

			page, err := explore(c, w.db, userData).Get()
			require.NoError(t, err)

			wantIDs := []string{}
			for _, profile := range row.profiles {
				wantIDs = append(wantIDs, w.posts[postKey{profile: profile, vis: core.PostVisibilityPublic, published: true}].ID)
			}

			gotIDs := []string{}
			for _, item := range page.Items {
				require.NotNil(t, item.Post)
				gotIDs = append(gotIDs, item.Post.ID)
				require.Equal(t, postops.PostCapabilities{}, *item.Post.Capabilities)
			}

			require.ElementsMatch(t, wantIDs, gotIDs)
		})
	}

	// A share link, /shared/:id, opens a published post for anyone who has
	// it, whatever the post's or the profile's visibility: that is what the
	// author creates it for. It never opens a draft.
	for key, share := range w.shares {
		for _, v := range []viewer{asAnonymous, asStranger} {
			state := "draft"
			if key.published {
				state = "published"
			}

			name := fmt.Sprintf("SharedPost/%s profile/%s %s post/%s", key.profile, key.vis, state, v)

			t.Run(name, func(t *testing.T) {
				t.Parallel()

				c, _ := w.userData(t, v, key.profile)
				page, err := shares.New(repo.Using(w.db)).Get(c, share.ID)

				if !key.published {
					require.ErrorIs(t, err, ginhelpers.ErrNotFound)
					return
				}

				require.NoError(t, err)
				require.Equal(t, w.posts[key].ID, page.Post.ID)
				require.Equal(t, w.authors[key.profile].ID, page.Author.ID)
			})
		}
	}

	t.Run("SharedPost/unknown share", func(t *testing.T) {
		t.Parallel()

		c, _ := w.userData(t, asAnonymous, core.ProfileVisibilityPublic)
		_, err := shares.New(repo.Using(w.db)).Get(c, uuid.NewString())
		require.ErrorIs(t, err, ginhelpers.ErrNotFound)
	})
}

var errPrivInjected = errors.New("injected database failure")

// failingExecutor passes queries through to db, except the failAt-th one,
// which fails.
type failingExecutor struct {
	db     *sqlx.DB
	failAt int
	calls  int
}

func (e *failingExecutor) fail() bool {
	e.calls++
	return e.calls == e.failAt
}

func (e *failingExecutor) Exec(query string, args ...any) (sql.Result, error) {
	return e.ExecContext(context.Background(), query, args...)
}

func (e *failingExecutor) Query(query string, args ...any) (*sql.Rows, error) {
	return e.QueryContext(context.Background(), query, args...)
}

func (e *failingExecutor) QueryRow(query string, args ...any) *sql.Row {
	return e.QueryRowContext(context.Background(), query, args...)
}

func (e *failingExecutor) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	if e.fail() {
		return nil, errPrivInjected
	}

	return e.db.ExecContext(ctx, query, args...)
}

func (e *failingExecutor) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	if e.fail() {
		return nil, errPrivInjected
	}

	return e.db.QueryContext(ctx, query, args...)
}

func (e *failingExecutor) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	if e.fail() {
		// a *sql.Row can't be built by hand; a query that can't run carries the error
		return e.db.QueryRowContext(ctx, "SELECT priv_injected_failure()")
	}

	return e.db.QueryRowContext(ctx, query, args...)
}

// A page that can't load everything it needs is an error, never a page
// with part of the privacy checks skipped: every query of every visible
// path is failed in turn, and each failure has to come back as an error.
func TestPrivacyMatrix_DatabaseFailuresAreErrors(t *testing.T) {
	t.Parallel()

	w := newWorld(t)
	publicAuthor := w.authors[core.ProfileVisibilityPublic]
	post := w.posts[postKey{profile: core.ProfileVisibilityPublic, vis: core.PostVisibilityPublic, published: true}]
	share := w.shares[postKey{profile: core.ProfileVisibilityPublic, vis: core.PostVisibilityPublic, published: true}]
	feedToken := testutil.Must(repo.RegenerateFeedToken(context.Background(), w.db, w.friend.ID))(t)

	cases := []struct {
		name   string
		viewer viewer
		call   func(c *gin.Context, exec boil.ContextExecutor, userData *auth.UserData) error
	}{
		{"SinglePost/author", asAuthor, func(c *gin.Context, exec boil.ContextExecutor, u *auth.UserData) error {
			return postPage(c, exec, u, post.ID).Error()
		}},
		{"SinglePost/second", asSecondDegree, func(c *gin.Context, exec boil.ContextExecutor, u *auth.UserData) error {
			return postPage(c, exec, u, post.ID).Error()
		}},
		{"UserHome/author", asAuthor, func(c *gin.Context, exec boil.ContextExecutor, u *auth.UserData) error {
			return userHome(c, exec, u, publicAuthor.Username).Error()
		}},
		{"UserHome/second", asSecondDegree, func(c *gin.Context, exec boil.ContextExecutor, u *auth.UserData) error {
			return userHome(c, exec, u, publicAuthor.Username).Error()
		}},
		{"Explore/anon", asAnonymous, func(c *gin.Context, exec boil.ContextExecutor, u *auth.UserData) error {
			return explore(c, exec, u).Error()
		}},
		{"SharedPost/anon", asAnonymous, func(c *gin.Context, exec boil.ContextExecutor, _ *auth.UserData) error {
			_, err := shares.New(repo.Using(exec)).Get(c, share.ID)
			return err
		}},
		{"Feed/direct", asDirect, func(c *gin.Context, exec boil.ContextExecutor, u *auth.UserData) error {
			_, err := reading.New(repo.Using(exec)).Feed(c, u.DBUser, false)
			return err
		}},
		// a failure after the author is found is an error, not a missing feed
		{"PublicFeed/anon", asAnonymous, func(c *gin.Context, exec boil.ContextExecutor, _ *auth.UserData) error {
			_, err := reading.New(repo.Using(exec)).PublicFeed(c, publicAuthor.Username)
			return err
		}},
		{"PrivateFeed/anon", asAnonymous, func(c *gin.Context, exec boil.ContextExecutor, _ *auth.UserData) error {
			_, err := reading.New(repo.Using(exec)).PrivateFeed(c, feedToken.Token)
			return err
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			for failAt := 1; ; failAt++ {
				c, userData := w.userData(t, tc.viewer, core.ProfileVisibilityPublic)
				exec := &failingExecutor{db: w.db, failAt: failAt}

				err := tc.call(c, exec, userData)

				if exec.calls < failAt {
					// every query has been failed once; with none failing, the page loads
					require.NoError(t, err)
					require.Greater(t, failAt, 1, "the page runs queries")

					return
				}

				require.Error(t, err, "query %d failed", failAt)
				require.NotErrorIs(t, err, ginhelpers.ErrNotFound, "query %d failed", failAt)
				require.NotErrorIs(t, err, ginhelpers.ErrNeedsLogin, "query %d failed", failAt)
			}
		})
	}
}
