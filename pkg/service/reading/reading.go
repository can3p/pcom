// Package reading decides who may read what: a single post with its comments,
// a user's journal, the explore page, the feed and the RSS feeds built from
// them. Every visibility rule of pcom is applied here, on top of the pure
// checks in postops.
package reading

import (
	"context"
	"errors"

	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/postops"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service"
	"github.com/can3p/pcom/pkg/service/graph"
)

type Service struct {
	store *repo.Store
}

func New(store *repo.Store) *Service {
	return &Service{store: store}
}

// Post is a post as the actor may see it: the comments and the share link
// are set only when the post's capabilities let the actor see them.
type Post struct {
	Post      *postops.Post
	Comments  []*postops.Comment
	PostShare *core.PostShare
}

// Post opens a post for the actor, nil for an anonymous visitor. A post the
// actor may not see is not found, so its existence isn't revealed, except
// that an anonymous visitor is asked to log in. editPreview marks the page
// the author previews while editing.
func (s *Service) Post(ctx context.Context, actor *core.User, postID string, editPreview bool) (*Post, error) {
	post, err := s.store.PostToRead(ctx, postID)
	if errors.Is(err, repo.ErrNotFound) {
		return nil, service.ErrNotFound
	} else if err != nil {
		return nil, err
	}

	radius, err := s.radius(ctx, actor, post.R.User.ID)
	if err != nil {
		return nil, err
	}

	if !postops.CanSeePost(post, radius) {
		if actor == nil {
			return nil, service.ErrNeedsLogin
		}

		return nil, service.ErrNotFound
	}

	out := &Post{Post: postops.ConstructPost(actor, post, radius, nil, editPreview)}

	if out.Post.Capabilities.CanViewComments {
		comments, err := s.store.CommentsOfPost(ctx, post.ID)
		if err != nil {
			return nil, err
		}

		out.Comments = postops.ConstructComments(actor, comments, radius)
	}

	if out.Post.Capabilities.CanShare {
		out.PostShare, err = s.store.ShareOfPost(ctx, post.ID)
		if err != nil {
			return nil, err
		}
	}

	return out, nil
}

// Journal is a user's profile page as the actor may see it.
type Journal struct {
	Author            *core.User
	ConnectionRadius  graph.Radius
	ConnectionAllowed bool
	MediationRequest  *core.UserConnectionMediationRequest
	Posts             []*postops.Post
	About             string // the author's "About" text, empty when there is none
	// Next is the cursor of the next page of posts, empty on the last one.
	Next string
}

// Journal opens the profile of the user with that username for the actor,
// nil for an anonymous visitor: the page after cursor (empty for the first)
// of the published posts the actor may read, and how the actor could connect
// to the author. A profile the actor may not see is not found, so whether it
// exists isn't revealed.
func (s *Service) Journal(ctx context.Context, actor *core.User, username, cursor string) (*Journal, error) {
	after, err := ParseCursor(cursor)
	if err != nil {
		return nil, err
	}

	return s.journal(ctx, actor, username, after, PageSize)
}

func (s *Service) journal(ctx context.Context, actor *core.User, username string, after Cursor, limit int) (*Journal, error) {
	author, err := s.store.UserByUsername(ctx, username)
	if errors.Is(err, repo.ErrNotFound) {
		return nil, service.ErrNotFound
	} else if err != nil {
		return nil, err
	}

	if CannotSeeProfileLite(author, actor) {
		return nil, service.ErrNotFound
	}

	radius, err := s.radius(ctx, actor, author.ID)
	if err != nil {
		return nil, err
	}

	if !CanSeeProfile(author, actor, radius) {
		return nil, service.ErrNotFound
	}

	var visibilities []core.PostVisibility

	switch radius {
	case graph.RadiusDirect, graph.RadiusSameUser:
		// direct connections and the author see every post
	case graph.RadiusSecondDegree:
		visibilities = []core.PostVisibility{core.PostVisibilitySecondDegree, core.PostVisibilityPublic}
	default:
		// anonymous and unrelated visitors, and any radius added later, get
		// public posts only
		visibilities = []core.PostVisibility{core.PostVisibilityPublic}
	}

	rawPosts, err := s.store.PublishedPostsOf(ctx, author.ID, visibilities, visibilities == nil, after.page(limit))
	if err != nil {
		return nil, err
	}

	about, err := s.store.ProfileAbout(ctx, author.ID)
	if err != nil {
		return nil, err
	}

	out := &Journal{About: about, Author: author, ConnectionRadius: radius}
	out.Posts, out.Next = postsPage(actor, rawPosts, radius, limit)

	if actor == nil {
		return out, nil
	}

	out.ConnectionAllowed, err = s.store.OpenGrantExists(ctx, author.ID, actor.ID)
	if err != nil {
		return nil, err
	}

	if radius == graph.RadiusSecondDegree {
		out.MediationRequest, err = s.store.MediationRequestBetween(ctx, actor.ID, author.ID)
		if errors.Is(err, repo.ErrNotFound) {
			out.MediationRequest = nil
		} else if err != nil {
			return nil, err
		}
	}

	return out, nil
}

// PublicFeed is what the public RSS feed of the user with that username
// lists: the journal an anonymous visitor sees. Only a public profile has
// one. It lists the newest RSSLimit posts.
func (s *Service) PublicFeed(ctx context.Context, username string) (*Journal, error) {
	author, err := s.store.UserByUsername(ctx, username)
	if errors.Is(err, repo.ErrNotFound) {
		return nil, service.ErrNotFound
	} else if err != nil {
		return nil, err
	}

	if author.ProfileVisibility != core.ProfileVisibilityPublic {
		return nil, service.ErrNotFound
	}

	return s.journal(ctx, nil, username, Cursor{}, RSSLimit)
}

// Posts is a page of posts. Next is the cursor of the next page, empty on
// the last one.
type Posts struct {
	Posts []*postops.Post
	Next  string
}

// Explore lists the page after cursor (empty for the first) of the published
// public posts of the profiles the actor could open without a connection:
// public ones, and for a logged in actor those open to registered users.
// Nobody gets comments or actions there.
func (s *Service) Explore(ctx context.Context, actor *core.User, cursor string) (*Posts, error) {
	profiles := []core.ProfileVisibility{core.ProfileVisibilityPublic}

	if actor != nil {
		// connections-only profiles are left out: their posts are in the
		// feed of those who may read them anyway
		profiles = append(profiles, core.ProfileVisibilityRegisteredUsers)
	}

	return s.publicPosts(ctx, actor, profiles, cursor, PageSize)
}

// PublicPosts is the page after cursor (empty for the first) of the public
// feed (Q15): the published public posts by authors whose profile is public,
// newest publication first. A public post of a profile open to registered
// users or connections only stays readable at its own page but is not listed
// here. Posts are built for nobody: no actor, no comments, no actions.
func (s *Service) PublicPosts(ctx context.Context, cursor string) (*Posts, error) {
	return s.publicPosts(ctx, nil, []core.ProfileVisibility{core.ProfileVisibilityPublic}, cursor, PageSize)
}

// PublicPostsRSS is the newest RSSLimit posts of the public feed, for its
// RSS output.
func (s *Service) PublicPostsRSS(ctx context.Context) ([]*postops.Post, error) {
	page, err := s.publicPosts(ctx, nil, []core.ProfileVisibility{core.ProfileVisibilityPublic}, "", RSSLimit)
	if err != nil {
		return nil, err
	}

	return page.Posts, nil
}

func (s *Service) publicPosts(ctx context.Context, actor *core.User, profiles []core.ProfileVisibility, cursor string, limit int) (*Posts, error) {
	after, err := ParseCursor(cursor)
	if err != nil {
		return nil, err
	}

	rawPosts, err := s.store.PublishedPostsByProfile(ctx, core.PostVisibilityPublic, profiles, after.page(limit))
	if err != nil {
		return nil, err
	}

	out := &Posts{}
	out.Posts, out.Next = postsPage(actor, rawPosts, graph.RadiusUnknown, limit)

	return out, nil
}

// postsPage builds the first limit posts of a repository page for the actor,
// and the cursor of the next page.
func postsPage(actor *core.User, rawPosts core.PostSlice, radius graph.Radius, limit int) ([]*postops.Post, string) {
	posts := make([]*postops.Post, 0, len(rawPosts))
	for _, p := range rawPosts {
		posts = append(posts, postops.ConstructPost(actor, p, radius, nil, false))
	}

	return cut(posts, limit, func(p *postops.Post) Cursor {
		return Cursor{Time: p.PublishedAt.Time, Kind: repo.KindPost, ID: p.ID}
	})
}

// radius is how far the author is from the actor; RadiusUnknown for an
// anonymous actor.
func (s *Service) radius(ctx context.Context, actor *core.User, authorID string) (graph.Radius, error) {
	var actorID string
	if actor != nil {
		actorID = actor.ID
	}

	radius, err := graph.RadiusBetween(ctx, s.store, actorID, authorID)
	if err != nil && !errors.Is(err, graph.ErrUserNotSignedIn) {
		return radius, err
	}

	return radius, nil
}
