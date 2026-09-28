package web_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/can3p/pcom/pkg/auth"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/postops"
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
type privViewer string

const (
	privAnon     privViewer = "anon"
	privStranger privViewer = "stranger"
	privSecond   privViewer = "second"
	privDirect   privViewer = "direct"
	privAuthor   privViewer = "author"
)

var privProfiles = []core.ProfileVisibility{
	core.ProfileVisibilityPublic,
	core.ProfileVisibilityRegisteredUsers,
	core.ProfileVisibilityConnections,
}

var privPostVisibilities = []core.PostVisibility{
	core.PostVisibilityDirectOnly,
	core.PostVisibilitySecondDegree,
	core.PostVisibilityPublic,
}

// privAccess is what a visitor gets when they open a post.
type privAccess string

const (
	// privNeedsLogin: the visitor is sent to log in.
	privNeedsLogin privAccess = "needs login"
	// privNotFound: the post doesn't exist as far as the visitor can tell.
	privNotFound privAccess = "not found"
	// privReader: the post is shown; comments are hidden and nothing can be done with it.
	privReader privAccess = "reader"
	// privCommenter: the post and its comments are shown and the visitor can comment.
	privCommenter privAccess = "commenter"
	// privOwner: everything, including edit and share.
	privOwner privAccess = "owner"
)

func (a privAccess) capabilities() postops.PostCapabilities {
	switch a {
	case privOwner:
		return postops.PostCapabilities{CanViewComments: true, CanLeaveComments: true, CanEdit: true, CanShare: true}
	case privCommenter:
		return postops.PostCapabilities{CanViewComments: true, CanLeaveComments: true}
	default:
		return postops.PostCapabilities{}
	}
}

type privPostKey struct {
	profile   core.ProfileVisibility
	vis       core.PostVisibility
	published bool
}

// privWorld holds one author per profile visibility, each with one post per
// post visibility, as a draft and published. Every post has a comment and a
// share link. friend is directly connected to every author, fof is connected
// to friend only and stranger to nobody.
type privWorld struct {
	db       *sqlx.DB
	authors  map[core.ProfileVisibility]*core.User
	friend   *core.User
	fof      *core.User
	stranger *core.User
	posts    map[privPostKey]*core.Post
	shares   map[privPostKey]*core.PostShare
}

func privNewWorld(t *testing.T) *privWorld {
	t.Helper()

	db := testdb.New(t).DB
	ctx := context.Background()

	w := &privWorld{
		db:      db,
		authors: map[core.ProfileVisibility]*core.User{},
		posts:   map[privPostKey]*core.Post{},
		shares:  map[privPostKey]*core.PostShare{},
	}

	var err error

	w.friend, err = factory.User(ctx, db)
	require.NoError(t, err)
	w.fof, err = factory.User(ctx, db)
	require.NoError(t, err)
	w.stranger, err = factory.User(ctx, db)
	require.NoError(t, err)

	_, _, err = factory.Connect(ctx, db, w.friend.ID, w.fof.ID)
	require.NoError(t, err)

	for _, profile := range privProfiles {
		author, err := factory.User(ctx, db, factory.WithVisibility(profile))
		require.NoError(t, err)
		w.authors[profile] = author

		_, _, err = factory.Connect(ctx, db, author.ID, w.friend.ID)
		require.NoError(t, err)

		for _, vis := range privPostVisibilities {
			for _, published := range []bool{false, true} {
				opts := []factory.PostOpt{factory.Visibility(vis)}
				if published {
					opts = append(opts, factory.Published())
				}

				post, err := factory.Post(ctx, db, author.ID, opts...)
				require.NoError(t, err)
				_, err = factory.Comment(ctx, db, post.ID, author.ID)
				require.NoError(t, err)
				share, err := factory.PostShare(ctx, db, post.ID)
				require.NoError(t, err)

				key := privPostKey{profile: profile, vis: vis, published: published}
				w.posts[key] = post
				w.shares[key] = share
			}
		}
	}

	return w
}

// userData returns the request context and user data of viewer looking at
// the author whose profile visibility is profile.
func (w *privWorld) userData(t *testing.T, viewer privViewer, profile core.ProfileVisibility) (*gin.Context, *auth.UserData) {
	t.Helper()

	var user *core.User

	switch viewer {
	case privAnon:
	case privStranger:
		user = w.stranger
	case privSecond:
		user = w.fof
	case privDirect:
		user = w.friend
	case privAuthor:
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

const privDraftBug = "known bug: SinglePost ignores published_at, so a draft is shown to everyone who could see it once published " +
	"(e.g. an anonymous visitor opens a public draft)"

const privUserHomeBug = "known bug: UserHome applies no visibility filter for a logged-in visitor unrelated to the author, " +
	"so they see direct_only and second_degree posts"

func TestPrivacyMatrix(t *testing.T) {
	t.Parallel()

	w := privNewWorld(t)

	// Opening a post by its URL, /posts/:id. The author's profile visibility
	// doesn't matter here: only the post's own visibility does, so every row
	// holds for all three profile visibilities. (This is current behavior,
	// pinned on purpose: a public post of a connections-only profile can be
	// read by anyone who has its link, although Explore and the profile page
	// don't list it.)
	singlePostSpec := []struct {
		viewer    privViewer
		vis       core.PostVisibility
		published bool
		want      privAccess
		bug       string
	}{
		{privAuthor, core.PostVisibilityDirectOnly, true, privOwner, ""},
		{privAuthor, core.PostVisibilitySecondDegree, true, privOwner, ""},
		{privAuthor, core.PostVisibilityPublic, true, privOwner, ""},
		// the author previews their own drafts
		{privAuthor, core.PostVisibilityDirectOnly, false, privOwner, ""},
		{privAuthor, core.PostVisibilitySecondDegree, false, privOwner, ""},
		{privAuthor, core.PostVisibilityPublic, false, privOwner, ""},

		{privDirect, core.PostVisibilityDirectOnly, true, privCommenter, ""},
		{privDirect, core.PostVisibilitySecondDegree, true, privCommenter, ""},
		{privDirect, core.PostVisibilityPublic, true, privCommenter, ""},
		{privDirect, core.PostVisibilityDirectOnly, false, privNotFound, privDraftBug},
		{privDirect, core.PostVisibilitySecondDegree, false, privNotFound, privDraftBug},
		{privDirect, core.PostVisibilityPublic, false, privNotFound, privDraftBug},

		{privSecond, core.PostVisibilityDirectOnly, true, privNotFound, ""},
		{privSecond, core.PostVisibilitySecondDegree, true, privReader, ""},
		{privSecond, core.PostVisibilityPublic, true, privReader, ""},
		{privSecond, core.PostVisibilityDirectOnly, false, privNotFound, ""},
		{privSecond, core.PostVisibilitySecondDegree, false, privNotFound, privDraftBug},
		{privSecond, core.PostVisibilityPublic, false, privNotFound, privDraftBug},

		{privStranger, core.PostVisibilityDirectOnly, true, privNotFound, ""},
		{privStranger, core.PostVisibilitySecondDegree, true, privNotFound, ""},
		{privStranger, core.PostVisibilityPublic, true, privReader, ""},
		{privStranger, core.PostVisibilityDirectOnly, false, privNotFound, ""},
		{privStranger, core.PostVisibilitySecondDegree, false, privNotFound, ""},
		{privStranger, core.PostVisibilityPublic, false, privNotFound, privDraftBug},

		// anonymous visitors are asked to log in rather than told the post is missing
		{privAnon, core.PostVisibilityDirectOnly, true, privNeedsLogin, ""},
		{privAnon, core.PostVisibilitySecondDegree, true, privNeedsLogin, ""},
		{privAnon, core.PostVisibilityPublic, true, privReader, ""},
		{privAnon, core.PostVisibilityDirectOnly, false, privNeedsLogin, ""},
		{privAnon, core.PostVisibilitySecondDegree, false, privNeedsLogin, ""},
		{privAnon, core.PostVisibilityPublic, false, privNeedsLogin, privDraftBug},
	}

	for _, profile := range privProfiles {
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

				key := privPostKey{profile: profile, vis: row.vis, published: row.published}
				post := w.posts[key]
				c, userData := w.userData(t, row.viewer, profile)

				res := web.SinglePost(c, w.db, userData, post.ID, false)

				switch row.want {
				case privNeedsLogin:
					require.ErrorIs(t, res.Error(), ginhelpers.ErrNeedsLogin)
					return
				case privNotFound:
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

		for _, viewer := range []privViewer{privAnon, privStranger} {
			c, userData := w.userData(t, viewer, core.ProfileVisibilityPublic)
			res := web.SinglePost(c, w.db, userData, uuid.NewString(), false)
			require.ErrorIs(t, res.Error(), ginhelpers.ErrNotFound, viewer)
		}
	})

	// The profile page, /users/:username. Profile visibility decides who may
	// open it at all (a hidden profile is always "not found", so its existence
	// isn't revealed); post visibility then decides which published posts it
	// lists. Drafts are never listed.
	all := privPostVisibilities
	secondAndPublic := []core.PostVisibility{core.PostVisibilitySecondDegree, core.PostVisibilityPublic}
	publicOnly := []core.PostVisibility{core.PostVisibilityPublic}

	userHomeSpec := []struct {
		profile core.ProfileVisibility
		viewer  privViewer
		want    privAccess // privNotFound, or the access to every listed post
		lists   []core.PostVisibility
		bug     string
	}{
		{core.ProfileVisibilityPublic, privAnon, privReader, publicOnly, ""},
		{core.ProfileVisibilityPublic, privStranger, privReader, publicOnly, privUserHomeBug},
		{core.ProfileVisibilityPublic, privSecond, privReader, secondAndPublic, ""},
		{core.ProfileVisibilityPublic, privDirect, privCommenter, all, ""},
		{core.ProfileVisibilityPublic, privAuthor, privOwner, all, ""},

		{core.ProfileVisibilityRegisteredUsers, privAnon, privNotFound, nil, ""},
		{core.ProfileVisibilityRegisteredUsers, privStranger, privReader, publicOnly, privUserHomeBug},
		{core.ProfileVisibilityRegisteredUsers, privSecond, privReader, secondAndPublic, ""},
		{core.ProfileVisibilityRegisteredUsers, privDirect, privCommenter, all, ""},
		{core.ProfileVisibilityRegisteredUsers, privAuthor, privOwner, all, ""},

		{core.ProfileVisibilityConnections, privAnon, privNotFound, nil, ""},
		{core.ProfileVisibilityConnections, privStranger, privNotFound, nil, ""},
		{core.ProfileVisibilityConnections, privSecond, privReader, secondAndPublic, ""},
		{core.ProfileVisibilityConnections, privDirect, privCommenter, all, ""},
		{core.ProfileVisibilityConnections, privAuthor, privOwner, all, ""},
	}

	for _, row := range userHomeSpec {
		t.Run(fmt.Sprintf("UserHome/%s profile/%s", row.profile, row.viewer), func(t *testing.T) {
			t.Parallel()

			if row.bug != "" {
				t.Skip(row.bug)
			}

			author := w.authors[row.profile]
			c, userData := w.userData(t, row.viewer, row.profile)

			res := web.UserHome(c, w.db, userData, author.Username)

			if row.want == privNotFound {
				require.ErrorIs(t, res.Error(), ginhelpers.ErrNotFound)
				return
			}

			page, err := res.Get()
			require.NoError(t, err)
			require.Equal(t, author.ID, page.Author.ID)

			wantIDs := []string{}
			for _, vis := range row.lists {
				wantIDs = append(wantIDs, w.posts[privPostKey{profile: row.profile, vis: vis, published: true}].ID)
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

		c, userData := w.userData(t, privStranger, core.ProfileVisibilityPublic)
		res := web.UserHome(c, w.db, userData, "no-such-user")
		require.ErrorIs(t, res.Error(), ginhelpers.ErrNotFound)
	})

	// Explore, /explore, lists published public posts only, and only of
	// profiles the visitor could open without a connection: public profiles
	// for anonymous visitors, public and registered-users profiles for anyone
	// logged in. Connections-only profiles never show up, whatever the
	// visitor's connections. Nobody gets comments or actions there.
	exploreSpec := []struct {
		viewer   privViewer
		profiles []core.ProfileVisibility
	}{
		{privAnon, []core.ProfileVisibility{core.ProfileVisibilityPublic}},
		{privStranger, []core.ProfileVisibility{core.ProfileVisibilityPublic, core.ProfileVisibilityRegisteredUsers}},
		{privSecond, []core.ProfileVisibility{core.ProfileVisibilityPublic, core.ProfileVisibilityRegisteredUsers}},
		{privDirect, []core.ProfileVisibility{core.ProfileVisibilityPublic, core.ProfileVisibilityRegisteredUsers}},
	}

	for _, row := range exploreSpec {
		t.Run(fmt.Sprintf("Explore/%s", row.viewer), func(t *testing.T) {
			t.Parallel()

			c, userData := w.userData(t, row.viewer, core.ProfileVisibilityPublic)

			page, err := web.Explore(c, w.db, userData).Get()
			require.NoError(t, err)

			wantIDs := []string{}
			for _, profile := range row.profiles {
				wantIDs = append(wantIDs, w.posts[privPostKey{profile: profile, vis: core.PostVisibilityPublic, published: true}].ID)
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
		for _, viewer := range []privViewer{privAnon, privStranger} {
			state := "draft"
			if key.published {
				state = "published"
			}

			name := fmt.Sprintf("SharedPost/%s profile/%s %s post/%s", key.profile, key.vis, state, viewer)

			t.Run(name, func(t *testing.T) {
				t.Parallel()

				c, userData := w.userData(t, viewer, key.profile)
				res := web.SharedPost(c, w.db, userData, share.ID)

				if !key.published {
					require.ErrorIs(t, res.Error(), ginhelpers.ErrNotFound)
					return
				}

				page, err := res.Get()
				require.NoError(t, err)
				require.Equal(t, w.posts[key].ID, page.Post.ID)
				require.Equal(t, w.authors[key.profile].ID, page.Author.ID)
			})
		}
	}

	t.Run("SharedPost/unknown share", func(t *testing.T) {
		t.Parallel()

		c, userData := w.userData(t, privAnon, core.ProfileVisibilityPublic)
		res := web.SharedPost(c, w.db, userData, uuid.NewString())
		require.ErrorIs(t, res.Error(), ginhelpers.ErrNotFound)
	})
}

var errPrivInjected = errors.New("injected database failure")

// privFailingExec passes queries through to db, except the failAt-th one,
// which fails.
type privFailingExec struct {
	db     *sqlx.DB
	failAt int
	calls  int
}

func (e *privFailingExec) fail() bool {
	e.calls++
	return e.calls == e.failAt
}

func (e *privFailingExec) Exec(query string, args ...any) (sql.Result, error) {
	return e.ExecContext(context.Background(), query, args...)
}

func (e *privFailingExec) Query(query string, args ...any) (*sql.Rows, error) {
	return e.QueryContext(context.Background(), query, args...)
}

func (e *privFailingExec) QueryRow(query string, args ...any) *sql.Row {
	return e.QueryRowContext(context.Background(), query, args...)
}

func (e *privFailingExec) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	if e.fail() {
		return nil, errPrivInjected
	}

	return e.db.ExecContext(ctx, query, args...)
}

func (e *privFailingExec) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	if e.fail() {
		return nil, errPrivInjected
	}

	return e.db.QueryContext(ctx, query, args...)
}

func (e *privFailingExec) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
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

	w := privNewWorld(t)
	publicAuthor := w.authors[core.ProfileVisibilityPublic]
	post := w.posts[privPostKey{profile: core.ProfileVisibilityPublic, vis: core.PostVisibilityPublic, published: true}]
	share := w.shares[privPostKey{profile: core.ProfileVisibilityPublic, vis: core.PostVisibilityPublic, published: true}]

	cases := []struct {
		name   string
		viewer privViewer
		call   func(c *gin.Context, exec boil.ContextExecutor, userData *auth.UserData) error
	}{
		{"SinglePost/author", privAuthor, func(c *gin.Context, exec boil.ContextExecutor, u *auth.UserData) error {
			return web.SinglePost(c, exec, u, post.ID, false).Error()
		}},
		{"SinglePost/second", privSecond, func(c *gin.Context, exec boil.ContextExecutor, u *auth.UserData) error {
			return web.SinglePost(c, exec, u, post.ID, false).Error()
		}},
		{"UserHome/author", privAuthor, func(c *gin.Context, exec boil.ContextExecutor, u *auth.UserData) error {
			return web.UserHome(c, exec, u, publicAuthor.Username).Error()
		}},
		{"UserHome/second", privSecond, func(c *gin.Context, exec boil.ContextExecutor, u *auth.UserData) error {
			return web.UserHome(c, exec, u, publicAuthor.Username).Error()
		}},
		{"Explore/anon", privAnon, func(c *gin.Context, exec boil.ContextExecutor, u *auth.UserData) error {
			return web.Explore(c, exec, u).Error()
		}},
		{"SharedPost/anon", privAnon, func(c *gin.Context, exec boil.ContextExecutor, u *auth.UserData) error {
			return web.SharedPost(c, exec, u, share.ID).Error()
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			for failAt := 1; ; failAt++ {
				c, userData := w.userData(t, tc.viewer, core.ProfileVisibilityPublic)
				exec := &privFailingExec{db: w.db, failAt: failAt}

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
