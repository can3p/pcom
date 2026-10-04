package reading

import (
	"context"
	"errors"
	"slices"

	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/postops"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service"
	"github.com/can3p/pcom/pkg/service/graph"
)

// PostToTranslate returns a published post the actor may read, for
// translation. Only logged-in readers translate: anonymous is ErrNeedsLogin.
// A draft, even the actor's own, and a post the actor may not see are
// ErrNotFound.
func (s *Service) PostToTranslate(ctx context.Context, actor *core.User, postID string) (*core.Post, error) {
	if actor == nil {
		return nil, service.ErrNeedsLogin
	}

	post, err := s.store.PostByID(ctx, postID)
	if errors.Is(err, repo.ErrNotFound) {
		return nil, service.ErrNotFound
	} else if err != nil {
		return nil, err
	}

	radius, err := s.radius(ctx, actor, post.UserID)
	if err != nil {
		return nil, err
	}

	if !post.PublishedAt.Valid || !postops.CanSeePost(post, radius) {
		return nil, service.ErrNotFound
	}

	return post, nil
}

// RSSItemToTranslate returns an RSS item of a feed the actor is subscribed
// to, the items their feed shows. Anonymous is ErrNeedsLogin; any other item
// is ErrNotFound.
func (s *Service) RSSItemToTranslate(ctx context.Context, actor *core.User, itemID string) (*core.RSSItem, error) {
	if actor == nil {
		return nil, service.ErrNeedsLogin
	}

	item, err := s.store.SubscribedRSSItem(ctx, actor.ID, itemID)
	if errors.Is(err, repo.ErrNotFound) {
		return nil, service.ErrNotFound
	}

	return item, err
}

// PostsToTranslate keeps the posts the actor may read and that are
// published, the rule of PostToTranslate for a batch. Anonymous keeps none.
func (s *Service) PostsToTranslate(ctx context.Context, actor *core.User, posts []*core.Post) ([]*core.Post, error) {
	if actor == nil {
		return nil, nil
	}

	// the radii of all authors come from the actor's two neighbourhood queries,
	// as the feed does, not one query per author
	direct, secondDegree, _, err := graph.DirectAndSecondDegree(ctx, s.store, actor.ID)
	if err != nil {
		return nil, err
	}

	radiusOf := func(authorID string) graph.Radius {
		switch {
		case authorID == actor.ID:
			return graph.RadiusSameUser
		case slices.Contains(direct, authorID):
			return graph.RadiusDirect
		case slices.Contains(secondDegree, authorID):
			return graph.RadiusSecondDegree
		default:
			return graph.RadiusUnrelated
		}
	}

	out := make([]*core.Post, 0, len(posts))

	for _, post := range posts {
		if post.PublishedAt.Valid && postops.CanSeePost(post, radiusOf(post.UserID)) {
			out = append(out, post)
		}
	}

	return out, nil
}
